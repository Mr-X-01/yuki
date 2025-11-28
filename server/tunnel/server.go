package tunnel

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	"yuki-server/client"
	"yuki-server/crypto"
	"yuki-server/proto"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type Server struct {
	proto.UnimplementedTunnelServiceServer
	clientManager *client.Manager
	sessions      map[string]*Session
	sessionsMutex sync.RWMutex
	sharedTunConn net.Conn
}

type Session struct {
	ClientID  string
	Cipher    *crypto.Cipher
	TunConn   net.Conn
	LastPing  time.Time
	BytesUp   int64
	BytesDown int64
}

func NewServer(clientManager *client.Manager) *Server {
	return &Server{
		clientManager: clientManager,
		sessions:      make(map[string]*Session),
	}
}

func NewServerWithTun(clientManager *client.Manager, sharedTun net.Conn) *Server {
	server := &Server{
		clientManager: clientManager,
		sessions:      make(map[string]*Session),
		sharedTunConn: sharedTun,
	}
	return server
}

// gRPC Connect method - main tunnel endpoint
func (s *Server) Connect(stream proto.TunnelService_ConnectServer) error {
	log.Println(" New client connection attempt")
	
	// Extract metadata
	md, ok := metadata.FromIncomingContext(stream.Context())
	if !ok {
		log.Println(" Missing metadata")
		return status.Errorf(codes.Unauthenticated, "missing metadata")
	}
	log.Println(" Metadata extracted")

	clientIDs := md.Get("client-id")
	secrets := md.Get("client-secret")

	if len(clientIDs) == 0 || len(secrets) == 0 {
		log.Println("❌ Missing credentials in metadata")
		return status.Errorf(codes.Unauthenticated, "missing credentials")
	}

	clientID := clientIDs[0]
	secret := secrets[0]
	log.Printf("📋 Client ID: %s", clientID[:8]+"...")

	// Authenticate client
	log.Println("🔐 Authenticating client...")
	if !s.clientManager.IsAuthorized(clientID, secret) {
		log.Println("❌ Authentication failed")
		return status.Errorf(codes.Unauthenticated, "invalid credentials")
	}
	log.Println("✅ Authentication successful")

	// Get client details
	client, exists := s.clientManager.GetClient(clientID)
	if !exists {
		log.Println("❌ Client not found")
		return status.Errorf(codes.NotFound, "client not found")
	}
	log.Printf("✅ Client loaded: %s", client.Name)

	// Generate encryption key
	key, err := crypto.GenerateKey()
	if err != nil {
		return status.Errorf(codes.Internal, "key generation failed")
	}

	cipher, err := crypto.NewServerCipher(key)
	if err != nil {
		return status.Errorf(codes.Internal, "cipher creation failed")
	}

	// Create TUN interface connection
	tunConn, err := s.createTunConnection()
	if err != nil {
		return status.Errorf(codes.Internal, "tun creation failed")
	}
	defer tunConn.Close()

	// Create session
	sessionID := fmt.Sprintf("%s-%d", clientID, time.Now().Unix())
	session := &Session{
		ClientID: clientID,
		Cipher:   cipher,
		TunConn:  tunConn,
		LastPing: time.Now(),
	}

	s.sessionsMutex.Lock()
	s.sessions[sessionID] = session
	s.sessionsMutex.Unlock()

	defer func() {
		s.sessionsMutex.Lock()
		delete(s.sessions, sessionID)
		s.sessionsMutex.Unlock()
		s.clientManager.SetActive(clientID, false)
	}()

	s.clientManager.SetActive(clientID, true)
	log.Printf("✅ Session created: %s", sessionID)

	// Send initial handshake with key
	handshakeFrame := &proto.TunnelFrame{
		Data:      key,
		Timestamp: time.Now().Unix(),
		SessionId: sessionID,
	}

	if err := stream.Send(handshakeFrame); err != nil {
		return err
	}

	// Start tunneling
	return s.handleTunneling(stream, session, client)
}

func (s *Server) handleTunneling(stream proto.TunnelService_ConnectServer, session *Session, client *client.Client) error {
	log.Println("🔄 Starting packet tunneling...")
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()

	// Goroutine for reading from gRPC stream and writing to TUN
	go func() {
		log.Println("📥 Started gRPC→TUN goroutine")
		defer cancel()
		packetCount := 0
		for {
			frame, err := stream.Recv()
			if err != nil {
				if err != io.EOF {
					log.Printf("❌ Stream recv error: %v", err)
				} else {
					log.Println("📪 Client closed stream")
				}
				return
			}
			
			packetCount++
			if packetCount%10 == 0 {
				log.Printf("📥 Received %d packets from client", packetCount)
			}

			// Decrypt frame data
			decryptedData, err := session.Cipher.Decrypt(frame.Data)
			if err != nil {
				log.Printf("Decryption error: %v", err)
				continue
			}

			// Parse custom frame
			customFrame, err := session.Cipher.DecryptFrame(decryptedData)
			if err != nil {
				log.Printf("Frame parsing error: %v", err)
				continue
			}

			switch customFrame.Type {
			case 0: // Data frame
				log.Printf("📝 Writing %d bytes to TUN", len(customFrame.Data))
				_, err := session.TunConn.Write(customFrame.Data)
				if err != nil {
					log.Printf("❌ TUN write error: %v", err)
					return
				}
				session.BytesDown += int64(len(customFrame.Data))

			case 1: // Ping frame
				session.LastPing = time.Now()
				// Send pong
				pongFrame := &crypto.Frame{Type: 2, Length: 0, Data: nil}
				pongData, err := session.Cipher.EncryptFrame(pongFrame)
				if err != nil {
					continue
				}

				encryptedPong, err := session.Cipher.Encrypt(pongData)
				if err != nil {
					continue
				}

				response := &proto.TunnelFrame{
					Data:      encryptedPong,
					Timestamp: time.Now().Unix(),
					SessionId: frame.SessionId,
				}
				stream.Send(response)

			case 2: // Pong frame
				session.LastPing = time.Now()
			}
		}
	}()

	// Main loop: read from TUN and send to gRPC stream
	buffer := make([]byte, 32768)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			session.TunConn.SetReadDeadline(time.Now().Add(time.Second))
			n, err := session.TunConn.Read(buffer)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					// Check for ping timeout
					if time.Since(session.LastPing) > 30*time.Second {
						return fmt.Errorf("ping timeout")
					}
					continue
				}
				if err != io.EOF {
					log.Printf("TUN read error: %v", err)
				}
				return err
			}

			// Check bandwidth limits
			if client.MaxBandwidth > 0 && session.BytesUp > client.MaxBandwidth {
				return fmt.Errorf("bandwidth limit exceeded")
			}

			// Create data frame
			dataFrame := &crypto.Frame{
				Type:   0,
				Length: uint32(n),
				Data:   buffer[:n],
			}

			frameData, err := session.Cipher.EncryptFrame(dataFrame)
			if err != nil {
				log.Printf("Frame encryption error: %v", err)
				continue
			}

			encryptedData, err := session.Cipher.Encrypt(frameData)
			if err != nil {
				log.Printf("Data encryption error: %v", err)
				continue
			}

			response := &proto.TunnelFrame{
				Data:      encryptedData,
				Timestamp: time.Now().Unix(),
				SessionId: session.ClientID,
			}

			if err := stream.Send(response); err != nil {
				return err
			}

			session.BytesUp += int64(n)
			s.clientManager.UpdateTraffic(session.ClientID, int64(n), 0)
		}
	}
}

// Fake legitimate gRPC endpoints for DPI evasion
func (s *Server) GetStatus(ctx context.Context, req *proto.StatusRequest) (*proto.StatusResponse, error) {
	return &proto.StatusResponse{
		Status:  "healthy",
		Uptime:  int64(time.Since(time.Now().Add(-24 * time.Hour)).Seconds()),
		Version: "1.2.3",
	}, nil
}

func (s *Server) GetMetrics(ctx context.Context, req *proto.MetricsRequest) (*proto.MetricsResponse, error) {
	metrics := map[string]float64{
		"cpu_usage":    45.2,
		"memory_usage": 62.8,
		"connections":  float64(len(s.sessions)),
		"uptime":       24.5,
	}

	return &proto.MetricsResponse{Values: metrics}, nil
}

// Create TUN interface connection
func (s *Server) createTunConnection() (net.Conn, error) {
	if s.sharedTunConn != nil {
		log.Println("🔗 Using shared TUN interface")
		return s.sharedTunConn, nil
	}
	
	return nil, fmt.Errorf("no TUN interface available - server must be initialized with NewServerWithTun")
}
