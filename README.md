# Ruuvi Listener

A macOS/Linux desktop app that scans for [Ruuvi Tag](https://ruuvi.com) Bluetooth sensors and sends temperature and humidity data to a [TRMNL](https://trmnl.com) custom plugin.

## Features

- Discovers nearby Ruuvi Tags automatically via Bluetooth LE
- Displays live temperature, humidity and "last seen" age for each tag
- Assign names to tags (persisted across restarts)
- Select which tags to include in each send
- Manually send data to your TRMNL display

## Requirements

- Go 1.22+
- macOS (Xcode command line tools) or Linux (BlueZ 5.48+)
- Bluetooth hardware
- A [TRMNL](https://trmnl.com) account with a Custom Plugin

## Setup

### 1. Install dependencies

**macOS**
```bash
xcode-select --install
```

**Linux**
```bash
sudo apt install bluez
```

### 2. Clone and configure

```bash
git clone <repo-url>
cd ruuvi-listener
cp config.json.example config.json
```

Edit `config.json` and paste in your TRMNL Custom Plugin webhook URL:

```json
{
  "webhook_url": "https://trmnl.com/api/custom_plugins/YOUR_PLUGIN_ID_HERE",
  "tags_file": "tags.json"
}
```

### 3. Build and run

```bash
go build -o ruuvi-listener .
./ruuvi-listener
```

On macOS you will be prompted to grant Bluetooth permission the first time.

## Usage

1. **Scanning** — the app starts scanning immediately on launch. Tags appear in the list as they are discovered.
2. **Naming** — click any row to open a rename dialog.
3. **Selecting** — use the checkbox on each row to choose which tags are sent to TRMNL.
4. **Sending** — press **Send to TRMNL** to push the current readings of all selected tags.

## TRMNL plugin template

The display layout for the TRMNL plugin is in `trmnl/layout.html`. Paste the contents of that file into the **Markup** field of your Custom Plugin in the TRMNL dashboard.

The template shows each tag's name, temperature, humidity and staleness status. Tags that have not reported in over 30 minutes are marked as stale; over 60 minutes as offline.

## Project structure

```
├── main.go                          Entry point
├── config.json.example              Config template (copy to config.json)
├── internal/
│   ├── service/
│   │   ├── bluetooth_service.go     BLE scanning
│   │   ├── ruuvi_parser.go          RAWv2 data format decoder
│   │   └── webhook_service.go       TRMNL HTTP POST
│   ├── model/
│   │   └── ruuvi_tag.go             Data model
│   └── ui/
│       └── app.go                   Fyne desktop UI
├── pkg/
│   └── storage/
│       └── store.go                 Tag persistence (tags.json)
└── trmnl/
    └── layout.html                  TRMNL plugin display template
```

## Data format

The app sends a `merge_variables` payload to the TRMNL webhook:

```json
{
  "merge_variables": {
    "ruuvi_tags": [
      {
        "name": "Living Room",
        "temperature": 22.3,
        "humidity": 48.0,
        "lastUpdated": "2026-03-24T20:41:00",
        "lastTemperatureUpdate": "2026-03-24T20:41:00"
      }
    ]
  }
}
```
