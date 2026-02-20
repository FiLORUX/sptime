// Package gpsdo provides an abstraction layer for GPS Disciplined Oscillators.
// It supports multiple GPSDO types including serial/PPS, network, and dummy for testing.
package gpsdo

import (
	"context"
	"fmt"
	"time"

	"github.com/sptime/sptime/internal/clock"
	"github.com/sptime/sptime/internal/config"
	"github.com/sptime/sptime/internal/logging"
)

// TimeReference is the interface that all GPSDO implementations must satisfy.
// It extends the clock.TimeReference interface with GPSDO-specific methods.
type TimeReference interface {
	// Core time reference methods
	Name() string
	GetTime() (time.Time, error)
	GetOffset() (time.Duration, error)
	IsLocked() bool
	Priority() int
	Quality() clock.ReferenceQuality

	// Lifecycle methods
	Start(ctx context.Context) error
	Stop()

	// GPSDO-specific methods
	GetStatus() Status
	GetSatellites() []Satellite
}

// Status represents the current GPSDO status.
type Status struct {
	Type            string        `json:"type"`
	Connected       bool          `json:"connected"`
	Locked          bool          `json:"locked"`
	LockDuration    time.Duration `json:"lock_duration"`
	Satellites      int           `json:"satellites"`
	SignalStrength  float64       `json:"signal_strength_db"`
	Offset          time.Duration `json:"offset_ns"`
	Latitude        float64       `json:"latitude,omitempty"`
	Longitude       float64       `json:"longitude,omitempty"`
	Altitude        float64       `json:"altitude_m,omitempty"`
	HDOP            float64       `json:"hdop,omitempty"`
	PPSCount        uint64        `json:"pps_count"`
	LastPPS         time.Time     `json:"last_pps"`
	LastNMEA        time.Time     `json:"last_nmea"`
	Firmware        string        `json:"firmware,omitempty"`
	SerialNumber    string        `json:"serial_number,omitempty"`
	Error           string        `json:"error,omitempty"`
}

// Satellite represents information about a single GPS satellite.
type Satellite struct {
	PRN       int     `json:"prn"`
	Elevation int     `json:"elevation"`
	Azimuth   int     `json:"azimuth"`
	SNR       float64 `json:"snr"`
	Used      bool    `json:"used"`
}

// New creates a new GPSDO instance based on the configuration.
func New(cfg config.GPSDOConfig, log *logging.Logger) (TimeReference, error) {
	switch cfg.Type {
	case "serialpps":
		return NewSerialPPS(cfg, log)
	case "network":
		return NewNetwork(cfg, log)
	case "dummy":
		return NewDummy(cfg, log)
	default:
		return nil, fmt.Errorf("unknown GPSDO type: %s", cfg.Type)
	}
}
