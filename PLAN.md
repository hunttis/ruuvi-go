# Ruuvi Tag Listener - Go Application Plan

## Overview

A cross-platform application that listens to Ruuvi Tags via Bluetooth, stores discovered tags, and sends weather data (temperature and humidity) to a remote webhook every 5 minutes.

## Target Platform

- **Device:** macOS (Mac Mini) or Linux server
- **Language:** Go
- **Build System:** Go modules

## Webhook Configuration

- **URL:** `https://trmnl.com/api/custom_plugins/fe151934-d989-4861-b0a1-dd228ab28eb8`
- **Method:** POST
- **Format:** JSON

## Data Format

```json
{
  "lastUpdated": "2026-03-22T07:14:49.033Z",
  "cache": {
    "d6113a2b907d": {
      "data": {
        "id": "d6113a2b",
        "name": null,
        "lastUpdated": "2026-02-22T23:07:25",
        "status": "active",
        "temperature": -5.68,
        "lastTemperatureUpdate": "2026-02-22T23:07:25",
        "humidity": 85.15
      }
    }
  }
}
```

## Architecture

**Pattern:** Clean Architecture with layered design

## Project Structure

```
ruuvi-listener/
├── main.go
├── go.mod
├── go.sum
├── internal/
│   ├── model/
│   │   ├── ruuvi_tag.go
│   │   └── weather_data.go
│   ├── service/
│   │   ├── bluetooth_service.go
│   │   ├── ruuvi_parser.go
│   │   └── webhook_service.go
│   └── config/
│       └── config.go
├── cmd/
│   └── ruuvi-listener/
│       └── main.go
└── pkg/
    ├── utils/
    │   └── logger.go
    └── storage/
        └── tag_store.go
```

## Core Components

| Component         | Technology                                            | Purpose                         |
| ----------------- | ----------------------------------------------------- | ------------------------------- |
| BLE Communication | gatt library or bluez (Linux) / CoreBluetooth (macOS) | Scan and listen for Ruuvi Tags  |
| Data Parsing      | Custom parser                                         | Decode Ruuvi Tag data format    |
| HTTP Client       | net/http or third-party library                       | POST data to remote server      |
| Timer/Scheduling  | time.Ticker                                           | Send data every 5 minutes       |
| Data Storage      | In-memory map or database                             | Store discovered tags (up to 5) |

## Key Features

1. **BLE Scanning:** Discover nearby Ruuvi Tags using platform-specific libraries
2. **Data Parsing:** Decode Ruuvi-specific data format (temperature, humidity)
3. **Tag Storage:** Store up to 5 tags of interest in memory
4. **Periodic Upload:** Send data to webhook every 5 minutes (minimum interval)
5. **Error Handling:** Skip sending if data collection fails
6. **UTC Timestamps:** All timestamps in UTC format

## Requirements

### System Requirements

- Go 1.21+
- macOS or Linux (with Bluetooth support)
- Network access for webhook communication

### Dependencies

- Go modules
- External BLE library (e.g., `github.com/paypal/gatt` or platform-specific)

### External Libraries

- BLE: `github.com/paypal/gatt` (cross-platform) or platform-specific
- Configuration: `github.com/spf13/viper` (optional)
- Logging: `github.com/sirupsen/logrus`

## Configuration

### Configurable Parameters

- Webhook URL (via environment variable or config file)
- Upload interval (default: 5 minutes, configurable via env var)
- Tag whitelist (optional)

### Runtime Behavior

- **Concurrency:** Use goroutines and channels
- **Error Handling:** Skip sending if errors occur
- **Timestamps:** UTC format (ISO 8601)
- **Logging:** Structured logging with timestamps

## Implementation Steps

1. **Project Setup**
   - Initialize Go module (`go mod init`)
   - Set up project structure
   - Add dependencies

2. **Models**
   - Define `RuuviTag` struct
   - Define `WeatherData` struct for JSON payload

3. **Bluetooth Service**
   - Implement BLE scanning using cross-platform library
   - Filter for Ruuvi Tag service UUIDs (0x181A)
   - Parse BLE advertisement data

4. **Ruuvi Parser**
   - Decode Ruuvi data format (URL scheme or raw data)
   - Extract temperature and humidity from advertisement data
   - Handle different Ruuvi Tag versions

5. **Webhook Service**
   - Implement HTTP POST request with net/http
   - Handle response and errors
   - Implement retry/skip logic

6. **Storage**
   - Implement in-memory tag storage (map[string]RuuviTag)
   - Add methods for adding/updating tags

7. **Main Application**
   - Set up configuration
   - Initialize services
   - Start BLE scanning in goroutine
   - Set up 5-minute timer for data uploads

8. **Testing**
   - Unit tests for parser
   - Integration tests for webhook
   - Mock BLE scanning for testing

## Concurrency Model

- Use goroutines for concurrent operations
- Channels for coordination between components
- Separate goroutine for BLE scanning
- Timer goroutine for periodic uploads

## Error Handling

| Scenario              | Action                                          |
| --------------------- | ----------------------------------------------- |
| BLE scan fails        | Log error, continue scanning                    |
| No tags discovered    | Skip sending, wait for next interval            |
| Webhook request fails | Log error, skip sending, wait for next interval |
| JSON encoding fails   | Log error, skip sending                         |

## Build and Deployment

### Building

```bash
go build -o ruuvi-listener ./cmd/ruuvi-listener
```

### Running

```bash
./ruuvi-listener --webhook-url="https://example.com/api" --interval=5m
```

### Environment Variables

- `RUUVII_LISTENER_WEBHOOK_URL` - Webhook URL
- `RUUVII_LISTENER_INTERVAL` - Upload interval (e.g., "5m", "10s")
- `RUUVII_LISTENER_DEBUG` - Enable debug logging

## Next Steps

1. Initialize Go module
2. Set up project structure
3. Implement models and data structures
4. Implement BLE scanning service
5. Implement Ruuvi parser
6. Implement webhook service
7. Implement main application loop
8. Write tests
9. Build and deploy

## Platform-Specific Notes

### macOS

- Use CoreBluetooth framework via CGO or native Go BLE library
- May need entitlements for Bluetooth access

### Linux

- Use BlueZ D-Bus interface or native Go library
- Requires bluetooth service running

### Cross-Platform Considerations

- Abstract BLE operations behind interface for easy platform switching
- Use feature detection to handle platform differences
