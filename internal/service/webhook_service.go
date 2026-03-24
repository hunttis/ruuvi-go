package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"ruuvi-listener/pkg/storage"
)

// tagEntry matches the fields the Liquid template accesses via `tag.*`.
type tagEntry struct {
	Name                      string  `json:"name"`
	Temperature               float64 `json:"temperature"`
	Humidity                  float64 `json:"humidity"`
	LastUpdated               string  `json:"lastUpdated"`               // RFC3339 UTC — used by Liquid for age calc
	LastTemperatureUpdate     string  `json:"lastTemperatureUpdate"`     // RFC3339 UTC — used by Liquid for age calc
	LastUpdatedTime           string  `json:"lastUpdatedTime"`           // local HH:MM — used directly for display
	LastTemperatureUpdateTime string  `json:"lastTemperatureUpdateTime"` // local HH:MM — used directly for display
}

type mergeVariables struct {
	RuuviTags []tagEntry `json:"ruuvi_tags"`
}

type webhookPayload struct {
	MergeVariables mergeVariables `json:"merge_variables"`
}

// WebhookService posts tag data to the configured TRMNL endpoint.
type WebhookService struct {
	url    string
	client *http.Client
}

// NewWebhookService creates a WebhookService for the given URL.
func NewWebhookService(url string) *WebhookService {
	return &WebhookService{
		url:    url,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Send posts all tags to the TRMNL webhook as a merge_variables payload.
func (w *WebhookService) Send(tags []*storage.Tag) error {
	entries := make([]tagEntry, 0, len(tags))
	for _, t := range tags {
		tsUTC := t.LastSeen.UTC().Format(time.RFC3339)
		tsLocal := t.LastSeen.Local().Format("15:04")
		entries = append(entries, tagEntry{
			Name:                      t.DisplayName(),
			Temperature:               t.Temperature,
			Humidity:                  t.Humidity,
			LastUpdated:               tsUTC,
			LastTemperatureUpdate:     tsUTC,
			LastUpdatedTime:           tsLocal,
			LastTemperatureUpdateTime: tsLocal,
		})
	}

	payload := webhookPayload{
		MergeVariables: mergeVariables{RuuviTags: entries},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("webhook: marshal: %w", err)
	}

	resp, err := w.client.Post(w.url, "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook: post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("webhook: server returned %d", resp.StatusCode)
	}

	return nil
}
