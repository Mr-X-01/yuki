package main

import (
	"log"
	"path/filepath"

	"yuki-server/config"
)

func main() {
	cfg := config.GenerateDefaultConfig()
	out := filepath.Join(".", "config.json")
	if err := cfg.SaveToFile(out); err != nil {
		log.Fatalf("Failed to save config: %v", err)
	}
	log.Println("✅ Default config generated:", out)
}
