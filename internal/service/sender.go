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

	mu             sync.RWMutex
	lastSent       time.Time
	nextSendAt     time.Time
	lastWebhookErr error // most recent webhook send error; nil on success
	webhookEnabled bool  // when false, webhook sends are skipped
	imageEnabled   bool  // when false, image sends are skipped
}

// NewSender creates a Sender. Pass a non-nil imageService to also send a
// rendered PNG image on each send. Call Start to begin the auto-send ticker.
func NewSender(webhook *WebhookService, imageService *WeatherImageService, store *storage.Store, interval time.Duration) *Sender {
	return &Sender{
		webhook:        webhook,
		imageService:   imageService,
		store:          store,
		interval:       interval,
		webhookEnabled: true,
		imageEnabled:   true,
	}
}

// SetWebhookEnabled enables or disables webhook (weather data) sends.
func (s *Sender) SetWebhookEnabled(v bool) {
	s.mu.Lock()
	s.webhookEnabled = v
	s.mu.Unlock()
}

// SetImageEnabled enables or disables weather image sends.
func (s *Sender) SetImageEnabled(v bool) {
	s.mu.Lock()
	s.imageEnabled = v
	s.mu.Unlock()
}

// WebhookEnabled reports whether webhook sends are currently enabled.
func (s *Sender) WebhookEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.webhookEnabled
}

// ImageEnabled reports whether image sends are currently enabled.
func (s *Sender) ImageEnabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.imageEnabled
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
				s.mu.RLock()
				wEnabled := s.webhookEnabled
				iEnabled := s.imageEnabled
				s.mu.RUnlock()

				if wEnabled {
					if err := s.webhook.Send(tags); err != nil {
						log.Printf("auto-send error: %v", err)
						s.mu.Lock()
						s.lastWebhookErr = err
						s.mu.Unlock()
					} else {
						s.mu.Lock()
						s.lastSent = time.Now()
						s.lastWebhookErr = nil
						s.mu.Unlock()
					}
				}
				if iEnabled && s.imageService != nil {
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

// Send dispatches tags to the enabled services immediately and records the
// send time on success. Used by the manual send button in the UI.
func (s *Sender) Send(tags []*storage.Tag) error {
	s.mu.RLock()
	wEnabled := s.webhookEnabled
	iEnabled := s.imageEnabled
	s.mu.RUnlock()

	var err error
	if wEnabled {
		err = s.webhook.Send(tags)
		s.mu.Lock()
		if err == nil {
			s.lastSent = time.Now()
			s.lastWebhookErr = nil
		} else {
			s.lastWebhookErr = err
		}
		s.mu.Unlock()
	}
	if iEnabled && s.imageService != nil {
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

// WebhookStatus returns a non-empty error string if the most recent webhook
// send failed, or an empty string if it succeeded or has never been attempted.
func (s *Sender) WebhookStatus() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.lastWebhookErr != nil {
		return "Error: " + s.lastWebhookErr.Error()
	}
	return ""
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

// ForceWebhookSend sends the webhook payload for the current selected tags
// immediately, regardless of the webhookEnabled flag.
func (s *Sender) ForceWebhookSend() error {
	tags := s.store.AllSelected()
	err := s.webhook.Send(tags)
	s.mu.Lock()
	if err == nil {
		s.lastSent = time.Now()
		s.lastWebhookErr = nil
	} else {
		s.lastWebhookErr = err
	}
	s.mu.Unlock()
	return err
}

// ForceImageSend renders and POSTs the weather image for the current selected
// tags immediately, regardless of the imageEnabled flag.
func (s *Sender) ForceImageSend() error {
	if s.imageService == nil {
		return nil
	}
	return s.imageService.Send(s.store.AllSelected())
}

// PreviewPayload builds the current webhook payload JSON from the selected tags
// without sending it or updating LastPayload.
func (s *Sender) PreviewPayload() (string, error) {
	return s.webhook.BuildPayload(s.store.AllSelected())
}

// LastSentImage returns the PNG bytes of the most recently successfully POSTed
// weather image, or nil if no image has been sent or image sends are disabled.
func (s *Sender) LastSentImage() []byte {
	if s.imageService == nil {
		return nil
	}
	return s.imageService.LastSentImage()
}
