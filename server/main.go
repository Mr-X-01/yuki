package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"yuki-server/api"
	"yuki-server/client"
	"yuki-server/config"
	"yuki-server/proto"
	"yuki-server/tunnel"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

var (
	configFile   = flag.String("config", "config.json", "Config file path")
	generateConf = flag.Bool("generate-config", false, "Generate default config")
)

func main() {
	flag.Parse()

	if *generateConf {
		generateDefaultConfig()
		return
	}

	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize client manager
	clientManager := client.NewManager()

	// Create TUN interface at startup
	log.Println("🔧 Creating TUN interface...")
	tunFile, err := tunnel.CreateTunInterface("tun0", "10.0.0.1/24", 1500)
	if err != nil {
		log.Fatalf("Failed to create TUN interface: %v", err)
	}
	tunConn := tunnel.NewTunConn(tunFile, "10.0.0.1", "10.0.0.2")
	log.Println("✅ Created TUN interface tun0 with IP 10.0.0.1")

	// Setup gRPC server with TLS
	creds, err := credentials.NewServerTLSFromFile(cfg.Server.CertFile, cfg.Server.KeyFile)
	if err != nil {
		log.Fatalf("Failed to load TLS credentials: %v", err)
	}

	grpcServer := grpc.NewServer(grpc.Creds(creds))
	
	// Register tunnel service with shared TUN connection
	tunnelServer := tunnel.NewServerWithTun(clientManager, tunConn)
	proto.RegisterTunnelServiceServer(grpcServer, tunnelServer)

	// Setup HTTP/REST API server
	apiServer := api.NewAPI(clientManager, cfg.Auth.AdminAPIKey, cfg.Auth.AdminLogin, cfg.Auth.AdminPassword)
	router := apiServer.SetupRoutes()

	// Start gRPC server (main service on port 443)
	grpcListener, err := net.Listen("tcp", fmt.Sprintf("%s:%d", cfg.Server.Address, cfg.Server.Port))
	if err != nil {
		log.Fatalf("Failed to listen on gRPC port: %v", err)
	}

	log.Printf("🚀 Yuki gRPC server starting on %s:%d", cfg.Server.Address, cfg.Server.Port)
	go func() {
		if err := grpcServer.Serve(grpcListener); err != nil {
			log.Fatalf("gRPC server failed: %v", err)
		}
	}()

	// Start HTTP API server (admin panel on port 8443)
	httpServer := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Server.Address, cfg.Server.AdminPort),
		Handler: router,
	}

	log.Printf("🌐 Yuki API server starting on %s:%d (HTTP - TLS on nginx)", cfg.Server.Address, cfg.Server.AdminPort)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("✅ Yuki VPN server is running")
	log.Println("🔒 gRPC tunnel service: port", cfg.Server.Port)
	log.Println("⚙️ Admin API: port", cfg.Server.AdminPort)

	<-sigChan
	log.Println("🛑 Shutting down servers...")

	grpcServer.GracefulStop()
	httpServer.Close()

	log.Println("✅ Shutdown complete")
}

func generateDefaultConfig() {
	cfg := config.GenerateDefaultConfig()
	
	if err := cfg.SaveToFile("config.json"); err != nil {
		log.Fatalf("Failed to save config: %v", err)
	}
	
	log.Println("✅ Default config generated: config.json")
	log.Println("📝 Don't forget to:")
	log.Println("   1. Update domain and SSL certificates")
	log.Println("   2. Change default API keys")
	log.Println("   3. Configure Redis if needed")
}
