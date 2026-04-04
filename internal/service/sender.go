package service

import (
	"log"
	"sync"
	"time"

	"ruuvi-listener/pkg/storage"
)

// Sender wraps WebhookService with auto-send scheduling and send-state tracking.
type Sender struct {
	webhook      *WebhookService
	imageService *WeatherImageService // optional; nil disables image sends
	store        *storage.Store
	interval     time.Duration

	mu         sync.RWMutex
	lastSent   time.Time
	nextSendAt time.Time
}

// NewSender creates a Sender. Pass a non-nil imageService to also send a
// rendered PNG image on each send. Call Start to begin the auto-send ticker.
func NewSender(webhook *WebhookService, imageService *WeatherImageService, store *storage.Store, interval time.Duration) *Sender {
	return &Sender{
		webhook:      webhook,
		imageService: imageService,
		store:        store,
		interval:     interval,
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
				if s.imageService != nil {
					if err := s.imageService.Send(tags); err != nil {
						log.Printf("auto image-send error: %v", err)
					}
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
	if s.imageService != nil {
		if imgErr := s.imageService.Send(tags); imgErr != nil {
			log.Printf("image send: %v", imgErr)
		}
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

// LastImage returns the PNG bytes of the most recently rendered weather image,
// or nil if no image has been rendered or image sends are disabled.
func (s *Sender) LastImage() []byte {
	if s.imageService == nil {
		return nil
	}
	return s.imageService.LastImage()
}

// LastImageSentAt returns the time the weather image was last successfully
// POSTed to the webhook, or the zero time if never sent or disabled.
func (s *Sender) LastImageSentAt() time.Time {
	if s.imageService == nil {
		return time.Time{}
	}
	return s.imageService.LastImageSentAt()
}

// ImageStatus returns a short human-readable status string for the image send.
func (s *Sender) ImageStatus() string {
	if s.imageService == nil {
		return ""
	}
	return s.imageService.ImageStatus()
}

// RenderImage renders the weather image from the current selected tags and
// caches it without sending. Returns nil if image sends are disabled.
func (s *Sender) RenderImage() error {
	if s.imageService == nil {
		return nil
	}
	return s.imageService.Render(s.store.AllSelected())
}
