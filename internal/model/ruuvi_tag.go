package model

import "time"

// RuuviTag represents a discovered Ruuvi Tag and its latest sensor data.
type RuuviTag struct {
	MAC         string    `json:"id"`
	Name        *string   `json:"name"`
	LastUpdated time.Time `json:"lastUpdated"`
	Status      string    `json:"status"`
	Temperature float64   `json:"temperature"`
	LastTemperatureUpdate time.Time `json:"lastTemperatureUpdate"`
	Humidity    float64   `json:"humidity"`
	Pressure    float64   `json:"pressure"`
}
