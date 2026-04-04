package service

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"ruuvi-listener/pkg/storage"
)

// WeatherImageService renders tag temperatures onto a PNG template and POSTs
// the result as image/png to a TRMNL image webhook.
type WeatherImageService struct {
	url          string
	templatePath string
	client       *http.Client

	mu         sync.RWMutex
	lastImage  []byte    // PNG bytes of the most recently rendered image
	lastSentAt time.Time // zero if never sent
}

// NewWeatherImageService creates a WeatherImageService.
func NewWeatherImageService(webhookURL, templatePath string) *WeatherImageService {
	return &WeatherImageService{
		url:          webhookURL,
		templatePath: templatePath,
		client:       &http.Client{Timeout: 15 * time.Second},
	}
}

// LastImage returns the PNG bytes of the most recently rendered image, or nil
// if no image has been rendered yet.
func (s *WeatherImageService) LastImage() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastImage
}

// LastImageSentAt returns the time the image was last successfully POSTed to
// the webhook, or the zero time if it has never been sent.
func (s *WeatherImageService) LastImageSentAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSentAt
}

// Render renders the image and caches it without sending it.
func (s *WeatherImageService) Render(tags []*storage.Tag) error {
	pngBytes, err := renderWeatherImage(s.templatePath, tags)
	if err != nil {
		return fmt.Errorf("weather image: render: %w", err)
	}
	s.mu.Lock()
	s.lastImage = pngBytes
	s.mu.Unlock()
	return nil
}

// Send renders the weather image and POSTs it to the webhook.
// If no webhook URL is configured it is a no-op (preview-only mode).
func (s *WeatherImageService) Send(tags []*storage.Tag) error {
	if s.url == "" {
		return nil
	}
	pngBytes, err := renderWeatherImage(s.templatePath, tags)
	if err != nil {
		return fmt.Errorf("weather image: render: %w", err)
	}

	s.mu.Lock()
	s.lastImage = pngBytes
	s.mu.Unlock()

	req, err := http.NewRequest("POST", s.url, bytes.NewReader(pngBytes))
	if err != nil {
		return fmt.Errorf("weather image: build request: %w", err)
	}
	req.Header.Set("Content-Type", "image/png")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("weather image: post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("weather image: server returned %d", resp.StatusCode)
	}

	s.mu.Lock()
	s.lastSentAt = time.Now()
	s.mu.Unlock()
	return nil
}

// renderWeatherImage loads the template PNG, draws temperatures, and returns the
// encoded PNG bytes.
func renderWeatherImage(templatePath string, tags []*storage.Tag) ([]byte, error) {
	raw, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("read template: %w", err)
	}

	src, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("decode png: %w", err)
	}

	// Ensure we have a writable *image.RGBA (png.Decode may return NRGBA etc.)
	dst := image.NewRGBA(src.Bounds())
	draw.Draw(dst, dst.Bounds(), src, image.Point{}, draw.Src)

	largeFace, err := loadFontFace(90)
	if err != nil {
		return nil, fmt.Errorf("load large font: %w", err)
	}
	medFace, err := loadFontFace(55)
	if err != nil {
		return nil, fmt.Errorf("load medium font: %w", err)
	}

	// Tag 0: outdoor — large, black, top-left, baseline at (50, 160)
	if len(tags) > 0 {
		drawAt(dst, largeFace, image.Black, tempStr(tags[0].Temperature), 50, 160)
	}

	// Tag 1: indoor (inside house) — medium, white, centered at x=455, baseline y=220
	if len(tags) > 1 {
		drawCentered(dst, medFace, image.White, tempStr(tags[1].Temperature), 455, 220)
	}

	// Tag 2: right of house — medium, black, baseline at (610, 220)
	if len(tags) > 2 {
		drawAt(dst, medFace, image.Black, tempStr(tags[2].Temperature), 610, 220)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}

// tempStr formats a temperature for display: one decimal + degree symbol.
func tempStr(t float64) string {
	return fmt.Sprintf("%.1f\u00b0", t)
}

// loadFontFace tries the macOS Helvetica Neue system font first, then falls
// back to the embedded Go Bold font.
func loadFontFace(sizePt float64) (font.Face, error) {
	const systemFont = "/System/Library/Fonts/HelveticaNeue.ttc"
	if data, err := os.ReadFile(systemFont); err == nil {
		coll, err := opentype.ParseCollection(data)
		if err == nil && coll.NumFonts() > 0 {
			f, err := coll.Font(0)
			if err == nil {
				face, err := opentype.NewFace(f, &opentype.FaceOptions{
					Size:    sizePt,
					DPI:     72,
					Hinting: font.HintingNone,
				})
				if err == nil {
					return face, nil
				}
			}
		}
	} else {
		log.Printf("weather image: system font unavailable (%v), using embedded Go Bold", err)
	}

	f, err := opentype.Parse(gobold.TTF)
	if err != nil {
		return nil, fmt.Errorf("parse embedded font: %w", err)
	}
	return opentype.NewFace(f, &opentype.FaceOptions{
		Size:    sizePt,
		DPI:     72,
		Hinting: font.HintingNone,
	})
}

// drawAt draws text left-aligned with its baseline at (x, baselineY).
func drawAt(dst *image.RGBA, face font.Face, src image.Image, text string, x, baselineY int) {
	d := &font.Drawer{
		Dst:  dst,
		Src:  src,
		Face: face,
		Dot:  fixed.P(x, baselineY),
	}
	d.DrawString(text)
}

// drawCentered draws text centered horizontally at centerX, baseline at baselineY.
func drawCentered(dst *image.RGBA, face font.Face, src image.Image, text string, centerX, baselineY int) {
	d := &font.Drawer{Dst: dst, Src: src, Face: face}
	advance := d.MeasureString(text)
	halfW := advance.Round() / 2
	d.Dot = fixed.P(centerX-halfW, baselineY)
	d.DrawString(text)
}

