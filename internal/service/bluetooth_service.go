package service

import (
	"log"
	"strings"

	"ruuvi-listener/pkg/storage"
	"tinygo.org/x/bluetooth"
)

// BLEService handles Bluetooth LE scanning for Ruuvi Tags.
type BLEService struct {
	adapter *bluetooth.Adapter
	store   *storage.Store
}

// NewBLEService creates a BLEService that writes discovered tags into store.
func NewBLEService(store *storage.Store) *BLEService {
	return &BLEService{
		adapter: bluetooth.DefaultAdapter,
		store:   store,
	}
}

// Start enables the BLE adapter and begins scanning. Blocks until Stop is
// called or a fatal error occurs.
func (s *BLEService) Start() error {
	if err := s.adapter.Enable(); err != nil {
		return err
	}

	log.Println("BLE: scanning for Ruuvi Tags…")

	return s.adapter.Scan(func(_ *bluetooth.Adapter, result bluetooth.ScanResult) {
		for _, md := range result.ManufacturerData() {
			if md.CompanyID != ruuviCompanyID {
				continue
			}

			parsed, err := ParseRuuviRAWv2(md.Data)
			if err != nil {
				log.Printf("BLE: parse error from %s: %v", result.Address, err)
				continue
			}

			mac := normalizeMAC(result.Address.String())
			s.store.UpdateFromBLE(mac, parsed.Temperature, parsed.Humidity, parsed.Pressure)
		}
	})
}

// Stop halts BLE scanning.
func (s *BLEService) Stop() error {
	return s.adapter.StopScan()
}

// normalizeMAC lowercases and strips colons: "D6:11:3A:2B:90:7D" → "d6113a2b907d".
func normalizeMAC(addr string) string {
	return strings.ReplaceAll(strings.ToLower(addr), ":", "")
}
