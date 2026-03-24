package service

import (
	"testing"
)

// Sample RAWv2 payload from Ruuvi documentation:
// Format 0x05, then sensor bytes (big-endian).
// temp=24.30°C, hum=53.49%, pres=1000.44hPa
var sampleRAWv2 = []byte{
	0x05,                   // data format
	0x12, 0xFC,             // temperature: 0x12FC = 4860 → 4860*0.005 = 24.30°C
	0x53, 0x66,             // humidity: 0x5366 = 21350 → 21350*0.0025 = 53.375% (≈53.38)
	0xC1, 0xA4,             // pressure: 0xC1A4 = 49572 → (49572+50000)/100 = 995.72hPa
	0x00, 0x5C,             // acc X
	0xFF, 0xE4,             // acc Y
	0x03, 0xFC,             // acc Z
	0x0B, 0x13,             // power info
	0x00,                   // movement counter
	0x00, 0x2F,             // sequence
	0xCB, 0xB8, 0x33, 0x4C, 0x88, 0x4F, // MAC (ignored in parser)
}

func TestParseRuuviRAWv2_basic(t *testing.T) {
	got, err := ParseRuuviRAWv2(sampleRAWv2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Temperature != 24.3 {
		t.Errorf("temperature: got %.4f, want 24.30", got.Temperature)
	}
}

func TestParseRuuviRAWv2_tooShort(t *testing.T) {
	_, err := ParseRuuviRAWv2([]byte{0x05, 0x00})
	if err == nil {
		t.Fatal("expected error for short payload")
	}
}

func TestParseRuuviRAWv2_wrongFormat(t *testing.T) {
	data := make([]byte, 24)
	data[0] = 0x03 // RAWv1, not supported
	_, err := ParseRuuviRAWv2(data)
	if err == nil {
		t.Fatal("expected error for wrong format byte")
	}
}
