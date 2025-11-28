package config

import (
	"encoding/json"
	"os"
)

type Config struct {
	ServerAddress string `json:"server_address"`
	ClientID      string `json:"client_id"`
	ClientSecret  string `json:"client_secret"`
	Protocol      string `json:"protocol"`
	Encryption    string `json:"encryption"`
	
	TunSettings struct {
		Name    string `json:"name"`
		IP      string `json:"ip"`
		Netmask string `json:"netmask"`
		Gateway string `json:"gateway"`
		DNS     []string `json:"dns"`
	} `json:"tun_settings"`
	
	Advanced struct {
		KeepAlive    int  `json:"keep_alive"`
		Reconnect    bool `json:"reconnect"`
		AutoStart    bool `json:"auto_start"`
		KillSwitch   bool `json:"kill_switch"`
	} `json:"advanced"`
}

func Load(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	cfg := &Config{}
	decoder := json.NewDecoder(file)
	return cfg, decoder.Decode(cfg)
}

func (c *Config) Save(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(c)
}

func Default() *Config {
	return &Config{
		ServerAddress: "api.example.ru:443",
		Protocol:      "grpc",
		Encryption:    "xchacha20-poly1305",
		TunSettings: struct {
			Name    string `json:"name"`
			IP      string `json:"ip"`
			Netmask string `json:"netmask"`
			Gateway string `json:"gateway"`
			DNS     []string `json:"dns"`
		}{
			Name:    "YukiVPN",
			IP:      "10.8.0.2",
			Netmask: "255.255.255.0",
			Gateway: "10.8.0.1",
			DNS:     []string{"1.1.1.1", "8.8.8.8"},
		},
		Advanced: struct {
			KeepAlive    int  `json:"keep_alive"`
			Reconnect    bool `json:"reconnect"`
			AutoStart    bool `json:"auto_start"`
			KillSwitch   bool `json:"kill_switch"`
		}{
			KeepAlive:  15,
			Reconnect:  true,
			AutoStart:  false,
			KillSwitch: false,
		},
	}
}
