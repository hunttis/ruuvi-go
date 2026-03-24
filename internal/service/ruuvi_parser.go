package service

import (
	"encoding/binary"
	"fmt"
	"math"
)

const ruuviCompanyID uint16 = 0x0499
const ruuviRAWv2Format byte = 0x05
const ruuviRAWv2Length = 24 // bytes after company ID

// RuuviRAWv2 holds decoded sensor values from a Ruuvi RAWv2 (Data Format 5) advertisement.
type RuuviRAWv2 struct {
	Temperature float64 // Celsius
	Humidity    float64 // %RH
	Pressure    float64 // hPa
}

// ParseRuuviRAWv2 decodes the manufacturer-specific data payload from a Ruuvi Tag
// RAWv2 advertisement. data must start with the format byte (0x05).
func ParseRuuviRAWv2(data []byte) (RuuviRAWv2, error) {
	if len(data) < ruuviRAWv2Length {
		return RuuviRAWv2{}, fmt.Errorf("ruuvi RAWv2: payload too short (%d < %d)", len(data), ruuviRAWv2Length)
	}
	if data[0] != ruuviRAWv2Format {
		return RuuviRAWv2{}, fmt.Errorf("ruuvi RAWv2: unexpected data format 0x%02x", data[0])
	}

	// Temperature: signed int16, resolution 0.005 °C
	rawTemp := int16(binary.BigEndian.Uint16(data[1:3]))
	temperature := round2(float64(rawTemp) * 0.005)

	// Humidity: unsigned uint16, resolution 0.0025 %RH
	rawHum := binary.BigEndian.Uint16(data[3:5])
	humidity := round2(float64(rawHum) * 0.0025)

	// Pressure: unsigned uint16, offset +50000 Pa, resolution 1 Pa → convert to hPa
	rawPres := binary.BigEndian.Uint16(data[5:7])
	pressure := round2(float64(rawPres+50000) / 100.0)

	return RuuviRAWv2{
		Temperature: temperature,
		Humidity:    humidity,
		Pressure:    pressure,
	}, nil
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
