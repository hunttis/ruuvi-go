package service

import (
	"log"
	"sync"
	"time"

	"ruuvi-listener/pkg/storage"
)

// Sender wraps WebhookService with auto-send scheduling and send-state tracking.
type Sender struct {
	webhook  *WebhookService
	store    *storage.Store
	interval time.Duration

	mu         sync.RWMutex
	lastSent   time.Time
	nextSendAt time.Time
}

// NewSender creates a Sender. Call Start to begin the auto-send ticker.
func NewSender(webhook *WebhookService, store *storage.Store, interval time.Duration) *Sender {
	return &Sender{
		webhook:  webhook,
		store:    store,
		interval: interval,
	}
}

// Start begins the auto-send ticker in a background goroutine.
func (s *Sender) Start() {
	s.mu.Lock()
	s.nextSendAt = time.Now().Add(s.interval)
	s.mu.Unlock()

	go func() {
		t := time.NewTicker(s.interval)
		defer t.Stop()
		for range t.C {
			tags := s.store.AllSelected()
			if len(tags) > 0 {
				if err := s.webhook.Send(tags); err != nil {
					log.Printf("auto-send error: %v", err)
				} else {
					s.mu.Lock()
					s.lastSent = time.Now()
					s.mu.Unlock()
				}
			}
			s.mu.Lock()
			s.nextSendAt = time.Now().Add(s.interval)
			s.mu.Unlock()
		}
	}()
}

// Send dispatches tags to the webhook immediately and records the send time on
// success. Used by the manual send button in the UI.
func (s *Sender) Send(tags []*storage.Tag) error {
	err := s.webhook.Send(tags)
	if err == nil {
		s.mu.Lock()
		s.lastSent = time.Now()
		s.mu.Unlock()
	}
	return err
}

// LastSent returns the time of the most recent successful send (zero if never sent).
func (s *Sender) LastSent() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSent
}

// NextSendAt returns the scheduled time of the next automatic send.
func (s *Sender) NextSendAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.nextSendAt
}

// LastPayload returns the pretty-printed JSON of the most recent send (empty if never sent).
func (s *Sender) LastPayload() string {
	return s.webhook.LastPayload()
}
