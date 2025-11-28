package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	Server struct {
		Address     string `json:"address"`
		Port        int    `json:"port"`
		AdminPort   int    `json:"admin_port"`
		CertFile    string `json:"cert_file"`
		KeyFile     string `json:"key_file"`
		Domain      string `json:"domain"`
	} `json:"server"`
	
	Redis struct {
		Address  string `json:"address"`
		Password string `json:"password"`
		DB       int    `json:"db"`
	} `json:"redis"`
	
	Auth struct {
		AdminAPIKey   string `json:"admin_api_key"`
		JWTSecret     string `json:"jwt_secret"`
		AdminLogin    string `json:"admin_login"`
		AdminPassword string `json:"admin_password"`
	} `json:"auth"`
	
	Tunnel struct {
		KeepAlive    int  `json:"keep_alive"`
		Compression  bool `json:"compression"`
		BufferSize   int  `json:"buffer_size"`
	} `json:"tunnel"`
	
	Limits struct {
		MaxClients     int   `json:"max_clients"`
		RateLimit      int   `json:"rate_limit"`
		MaxBandwidth   int64 `json:"max_bandwidth"`
	} `json:"limits"`
}

func LoadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	cfg := &Config{}
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func GenerateDefaultConfig() *Config {
	return &Config{
		Server: struct {
			Address     string `json:"address"`
			Port        int    `json:"port"`
			AdminPort   int    `json:"admin_port"`
			CertFile    string `json:"cert_file"`
			KeyFile     string `json:"key_file"`
			Domain      string `json:"domain"`
		}{
			Address:   "0.0.0.0",
			Port:      443,
			AdminPort: 8443,
			CertFile:  "/etc/ssl/certs/yuki.crt",
			KeyFile:   "/etc/ssl/private/yuki.key",
			Domain:    "api.example.ru",
		},
		Redis: struct {
			Address  string `json:"address"`
			Password string `json:"password"`
			DB       int    `json:"db"`
		}{
			Address:  "localhost:6379",
			Password: "",
			DB:       0,
		},
		Auth: struct {
			AdminAPIKey string `json:"admin_api_key"`
			JWTSecret   string `json:"jwt_secret"`
			AdminLogin    string `json:"admin_login"`
			AdminPassword string `json:"admin_password"`
		}{
			AdminAPIKey: "change-me-admin-key-2025",
			JWTSecret:   "change-me-jwt-secret-2025",
			AdminLogin:    "admin",
			AdminPassword: "password",
		},
		Tunnel: struct {
			KeepAlive    int  `json:"keep_alive"`
			Compression  bool `json:"compression"`
			BufferSize   int  `json:"buffer_size"`
		}{
			KeepAlive:   15,
			Compression: false,
			BufferSize:  32768,
		},
		Limits: struct {
			MaxClients     int   `json:"max_clients"`
			RateLimit      int   `json:"rate_limit"`
			MaxBandwidth   int64 `json:"max_bandwidth"`
		}{
			MaxClients:   1000,
			RateLimit:    100,
			MaxBandwidth: 1073741824, // 1GB
		},
	}
}

func (c *Config) SaveToFile(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(c)
}
