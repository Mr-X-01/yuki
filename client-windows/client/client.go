package client

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"time"

	"yuki-client/config"
	"yuki-client/crypto"
	"yuki-client/tun"
)

type Client struct {
	config     *config.Config
	tunIface   tun.Interface
	conn       net.Conn
	cipher     *crypto.Cipher
	connected  bool
	stats      Stats
}

type Stats struct {
	BytesUp   int64
	BytesDown int64
	Connected time.Time
	LastSeen  time.Time
}

func New(cfg *config.Config) *Client {
	return &Client{
		config:    cfg,
		connected: false,
	}
}

func (c *Client) Connect() error {
	// Create cipher key from config
	key := sha256.Sum256([]byte(c.config.Encryption))
	cipher, err := crypto.NewCipher(key[:])
	if err != nil {
		return fmt.Errorf("cipher creation failed: %w", err)
	}
	c.cipher = cipher

	// Create TUN interface
	tunIface := tun.NewTunInterface()
	if err := tunIface.Create("Yuki Tunnel"); err != nil {
		return fmt.Errorf("TUN interface creation failed: %w", err)
	}
	c.tunIface = tunIface

	// Connect to server via TCP
	conn, err := net.Dial("tcp", c.config.ServerAddress)
	if err != nil {
		tunIface.Close()
		return fmt.Errorf("connection failed: %w", err)
	}
	c.conn = conn

	// Send authentication
	authMsg := map[string]string{
		"client_id":     c.config.ClientID,
		"client_secret": c.config.ClientSecret,
	}

	authData, _ := json.Marshal(authMsg)
	if _, err := conn.Write(authData); err != nil {
		conn.Close()
		tunIface.Close()
		return fmt.Errorf("auth send failed: %w", err)
	}

	c.connected = true
	c.stats.Connected = time.Now()

	// Start packet relay
	go c.relayPackets()

	return nil
}

func (c *Client) relayPackets() {
	defer func() {
		c.connected = false
	}()

	buffer := make([]byte, 4096)
	for c.connected {
		// Read from TUN
		n, err := c.tunIface.Read(buffer)
		if err != nil {
			break
		}

		// Encrypt and send to server
		encrypted, err := c.cipher.Encrypt(buffer[:n])
		if err != nil {
			break
		}
		if _, err := c.conn.Write(encrypted); err != nil {
			break
		}

		c.stats.BytesUp += int64(len(encrypted))

		// Read response from server
		respBuffer := make([]byte, 4096)
		c.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		m, err := c.conn.Read(respBuffer)
		if err != nil {
			break
		}

		// Decrypt and write to TUN
		decrypted, err := c.cipher.Decrypt(respBuffer[:m])
		if err != nil {
			break
		}
		if _, err := c.tunIface.Write(decrypted); err != nil {
			break
		}

		c.stats.BytesDown += int64(len(decrypted))
	}
}

func (c *Client) IsConnected() bool {
	return c.connected
}

func (c *Client) GetStats() Stats {
	return c.stats
}

func (c *Client) Disconnect() error {
	c.connected = false
	
	if c.conn != nil {
		c.conn.Close()
	}
	
	if c.tunIface != nil {
		c.tunIface.Close()
	}
	
	return nil
}
