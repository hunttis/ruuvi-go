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
	SendInterval string `json:"send_interval"`
	// tags_file intentionally removed — always stored in Application Support
}

// appSupportDir returns ~/Library/Application Support/RuuviListener, creating it if needed.
// This directory survives app rebuilds and reinstalls.
func appSupportDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	dir := filepath.Join(home, "Library", "Application Support", "RuuviListener")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Printf("Warning: could not create app support dir: %v", err)
	}
	return dir
}

// bundleResourcesDir returns the Resources directory when running inside a
// .app bundle, or the current working directory otherwise.
func bundleResourcesDir() string {
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
	support := appSupportDir()

	// Look for config.json in Application Support first so it survives rebuilds.
	// Fall back to the bundle resources / working directory.
	cfgPath := filepath.Join(support, "config.json")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		cfgPath = filepath.Join(bundleResourcesDir(), "config.json")
	}

	cfg, err := loadConfig(cfgPath)
	if err != nil {
		log.Fatalf("Failed to read config.json: %v\n"+
			"Place config.json in %s or in the app bundle Resources folder.", err, support)
	}
	log.Printf("Config loaded from: %s", cfgPath)

	// Tags always live in Application Support — not configurable, survives rebuilds.
	tagsFile := filepath.Join(support, "tags.json")
	log.Printf("Tags file: %s", tagsFile)

	interval := 10 * time.Minute
	if cfg.SendInterval != "" {
		if d, err := time.ParseDuration(cfg.SendInterval); err == nil {
			interval = d
		} else {
			log.Printf("Invalid send_interval %q, using default 10m", cfg.SendInterval)
		}
	}

	store, err := storage.NewStore(tagsFile)
	if err != nil {
		log.Fatalf("Failed to load tag store: %v", err)
	}

	fmi := service.NewFmiCollector("Helsinki")
	webhook := service.NewWebhookService(cfg.WebhookURL, fmi)
	sender := service.NewSender(webhook, store, interval)
	sender.Start()

	// Fetch FMI forecast immediately so weather data is ready before the first send.
	go func() {
		if _, err := fmi.Get(); err != nil {
			log.Printf("FMI initial fetch failed: %v", err)
		} else {
			log.Printf("FMI initial fetch succeeded")
		}
	}()

	ble := service.NewBLEService(store)
	go func() {
		if err := ble.Start(); err != nil {
			log.Printf("BLE error: %v", err)
		}
	}()

	// UI blocks on the main goroutine (required by Fyne / CoreBluetooth).
	ui.Run(store, sender, fmi)
}
