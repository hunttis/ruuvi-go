package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ruuvi-listener/internal/service"
	"ruuvi-listener/internal/ui"
	"ruuvi-listener/pkg/storage"
)

type config struct {
	WebhookURL   string `json:"webhook_url"`
	TagsFile     string `json:"tags_file"`
	SendInterval string `json:"send_interval"`
}

// configDir returns the directory where config files live.
// When running inside a .app bundle the executable is at Contents/MacOS/;
// config files should be placed in Contents/Resources/.
// When running from the command line it falls back to the working directory.
func configDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	dir := filepath.Dir(exe)
	if strings.HasSuffix(dir, "/Contents/MacOS") {
		resources := filepath.Join(dir, "../Resources")
		if _, err := os.Stat(resources); err == nil {
			return resources
		}
	}
	return "."
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
	base := configDir()
	cfg, err := loadConfig(filepath.Join(base, "config.json"))
	if err != nil {
		log.Fatalf("Failed to read config.json: %v\nCopy config.json.example to config.json and fill in your details.", err)
	}
	if cfg.TagsFile == "" {
		cfg.TagsFile = filepath.Join(base, "tags.json")
	}

	interval := 10 * time.Minute
	if cfg.SendInterval != "" {
		if d, err := time.ParseDuration(cfg.SendInterval); err == nil {
			interval = d
		} else {
			log.Printf("Invalid send_interval %q, using default 10m", cfg.SendInterval)
		}
	}

	store, err := storage.NewStore(cfg.TagsFile)
	if err != nil {
		log.Fatalf("Failed to load tag store: %v", err)
	}

	ble := service.NewBLEService(store)
	webhook := service.NewWebhookService(cfg.WebhookURL)
	sender := service.NewSender(webhook, store, interval)
	sender.Start()

	// BLE scanning blocks, so run it in a goroutine.
	go func() {
		if err := ble.Start(); err != nil {
			log.Printf("BLE error: %v", err)
		}
	}()

	// UI blocks on the main goroutine (required by Fyne / CoreBluetooth).
	ui.Run(store, sender)
}
