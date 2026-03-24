package main

import (
	"encoding/json"
	"log"
	"os"

	"ruuvi-listener/internal/service"
	"ruuvi-listener/internal/ui"
	"ruuvi-listener/pkg/storage"
)

type config struct {
	WebhookURL string `json:"webhook_url"`
	TagsFile   string `json:"tags_file"`
}

func loadConfig(path string) (config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return config{}, err
	}
	var cfg config
	return cfg, json.Unmarshal(data, &cfg)
}

func main() {
	cfg, err := loadConfig("config.json")
	if err != nil {
		log.Fatalf("Failed to read config.json: %v\nCopy config.json.example to config.json and fill in your details.", err)
	}
	if cfg.TagsFile == "" {
		cfg.TagsFile = "tags.json"
	}

	store, err := storage.NewStore(cfg.TagsFile)
	if err != nil {
		log.Fatalf("Failed to load tag store: %v", err)
	}

	ble := service.NewBLEService(store)
	webhook := service.NewWebhookService(cfg.WebhookURL)

	// BLE scanning blocks, so run it in a goroutine.
	go func() {
		if err := ble.Start(); err != nil {
			log.Printf("BLE error: %v", err)
		}
	}()

	// UI blocks on the main goroutine (required by Fyne / CoreBluetooth).
	ui.Run(store, webhook)
}
