package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"ruuvi-listener/pkg/storage"
)

// tagEntry matches the fields the Liquid template accesses via `tag.*`.
type tagEntry struct {
	Name                      string  `json:"name"`
	Temperature               float64 `json:"temperature"`
	Humidity                  float64 `json:"humidity"`
	LastUpdated               int64   `json:"lastUpdated"`               // Unix epoch seconds — used by Liquid for age calc
	LastTemperatureUpdate     int64   `json:"lastTemperatureUpdate"`     // Unix epoch seconds — used by Liquid for age calc
	LastUpdatedTime           string  `json:"lastUpdatedTime"`           // local HH:MM — used directly for display
	LastTemperatureUpdateTime string  `json:"lastTemperatureUpdateTime"` // local HH:MM — used directly for display
}

type mergeVariables struct {
	RuuviTags       []tagEntry     `json:"ruuvi_tags"`
	WeatherCurrent  *ForecastHour  `json:"weather_current,omitempty"`
	WeatherForecast []ForecastHour `json:"weather_forecast,omitempty"`
}

type webhookPayload struct {
	MergeVariables mergeVariables `json:"merge_variables"`
}

// WebhookService posts tag data to the configured TRMNL endpoint.
type WebhookService struct {
	url     string
	client  *http.Client
	weather *FmiCollector // optional; nil disables weather in payload

	mu          sync.RWMutex
	lastPayload string // pretty-printed JSON of the last successfully marshalled send
}

// NewWebhookService creates a WebhookService for the given URL.
// Pass a non-nil FmiCollector to include weather data in each send.
func NewWebhookService(url string, weather *FmiCollector) *WebhookService {
	return &WebhookService{
		url:     url,
		client:  &http.Client{Timeout: 10 * time.Second},
		weather: weather,
	}
}

// LastPayload returns the pretty-printed JSON of the most recent send (empty if never sent).
func (w *WebhookService) LastPayload() string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastPayload
}

// BuildPayload constructs the full JSON payload for the given tags (including
// weather data if available) without POSTing it or updating LastPayload.
func (w *WebhookService) BuildPayload(tags []*storage.Tag) (string, error) {
	entries := make([]tagEntry, 0, len(tags))
	for _, t := range tags {
		tsUnix := t.LastSeen.Unix()
		tsLocal := t.LastSeen.Local().Format("15:04")
		entries = append(entries, tagEntry{
			Name:                      t.DisplayName(),
			Temperature:               t.Temperature,
			Humidity:                  t.Humidity,
			LastUpdated:               tsUnix,
			LastTemperatureUpdate:     tsUnix,
			LastUpdatedTime:           tsLocal,
			LastTemperatureUpdateTime: tsLocal,
		})
	}

	mv := mergeVariables{RuuviTags: entries}

	if w.weather != nil {
		wd, err := w.weather.Get()
		if err == nil {
			mv.WeatherCurrent = &wd.Current
			mv.WeatherForecast = wd.Forecast
		}
	}

	pretty, err := json.MarshalIndent(webhookPayload{MergeVariables: mv}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("webhook: marshal: %w", err)
	}
	return string(pretty), nil
}

// Send posts all tags (and optional weather) to the TRMNL webhook.
func (w *WebhookService) Send(tags []*storage.Tag) error {
	entries := make([]tagEntry, 0, len(tags))
	for _, t := range tags {
		tsUnix := t.LastSeen.Unix()
		tsLocal := t.LastSeen.Local().Format("15:04")
		entries = append(entries, tagEntry{
			Name:                      t.DisplayName(),
			Temperature:               t.Temperature,
			Humidity:                  t.Humidity,
			LastUpdated:               tsUnix,
			LastTemperatureUpdate:     tsUnix,
			LastUpdatedTime:           tsLocal,
			LastTemperatureUpdateTime: tsLocal,
		})
	}

	mv := mergeVariables{RuuviTags: entries}

	if w.weather != nil {
		wd, err := w.weather.Get()
		if err != nil {
			log.Printf("webhook: weather unavailable: %v", err)
		} else {
			mv.WeatherCurrent = &wd.Current
			mv.WeatherForecast = wd.Forecast
		}
	}

	payload := webhookPayload{MergeVariables: mv}

	pretty, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("webhook: marshal: %w", err)
	}

	w.mu.Lock()
	w.lastPayload = string(pretty)
	w.mu.Unlock()

	resp, err := w.client.Post(w.url, "application/json", bytes.NewReader(pretty))
	if err != nil {
		return fmt.Errorf("webhook: post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook: server returned %d", resp.StatusCode)
	}

	return nil
}
