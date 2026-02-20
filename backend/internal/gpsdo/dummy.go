// Package gpsdo provides a Dummy implementation for testing and development.
package gpsdo

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"

	"github.com/sptime/sptime/internal/clock"
	"github.com/sptime/sptime/internal/config"
	"github.com/sptime/sptime/internal/logging"
)

// Dummy implements a simulated GPSDO for testing purposes.
type Dummy struct {
	mu sync.RWMutex

	cfg config.GPSDOConfig
	log *logging.Logger

	running    bool
	locked     bool
	lockStart  time.Time

	// Simulated data
	satellites []Satellite
	offset     time.Duration
	ppsCount   uint64
	lastPPS    time.Time

	// Simulation parameters
	baseOffset   float64 // Base offset in nanoseconds
	driftRate    float64 // Drift in ns/s
	noiseLevel   float64 // Random noise in ns
}

// NewDummy creates a new Dummy GPSDO instance.
func NewDummy(cfg config.GPSDOConfig, log *logging.Logger) (*Dummy, error) {
	return &Dummy{
		cfg:        cfg,
		log:        log.WithField("gpsdo", "dummy"),
		locked:     true,
		baseOffset: 0,
		driftRate:  0.1,     // 0.1 ns/s drift
		noiseLevel: 100,     // 100 ns noise
		satellites: generateDummySatellites(),
	}, nil
}

// generateDummySatellites creates simulated satellite data.
func generateDummySatellites() []Satellite {
	sats := make([]Satellite, 0, 12)
	for i := 0; i < 12; i++ {
		prn := i + 1
		if rand.Intn(3) == 0 {
			prn += 32 // GLONASS
		}
		sats = append(sats, Satellite{
			PRN:       prn,
			Elevation: 15 + rand.Intn(60),
			Azimuth:   rand.Intn(360),
			SNR:       30 + rand.Float64()*15,
			Used:      i < 8,
		})
	}
	return sats
}

// Name returns the reference name.
func (d *Dummy) Name() string {
	return "GPSDO:Dummy"
}

// Start starts the dummy GPSDO simulation.
func (d *Dummy) Start(ctx context.Context) error {
	d.mu.Lock()
	d.running = true
	d.lockStart = time.Now()
	d.mu.Unlock()

	d.log.Info("Dummy GPSDO started")

	ticker := time.NewTicker(d.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			d.simulate()
		}
	}
}

// simulate updates the simulated GPSDO state.
func (d *Dummy) simulate() {
	d.mu.Lock()
	defer d.mu.Unlock()

	now := time.Now()

	// Simulate PPS
	d.ppsCount++
	d.lastPPS = now

	// Simulate offset with drift and noise
	elapsed := now.Sub(d.lockStart).Seconds()
	drift := d.driftRate * elapsed
	noise := (rand.Float64() - 0.5) * 2 * d.noiseLevel
	offsetNs := d.baseOffset + drift + noise

	d.offset = time.Duration(offsetNs) * time.Nanosecond

	// Occasionally simulate satellite changes
	if rand.Intn(60) == 0 {
		d.updateSatellites()
	}

	// Simulate occasional lock loss (1 in 1000 polls)
	if rand.Intn(1000) == 0 && d.locked {
		d.locked = false
		d.log.Warn("Simulated lock loss")
	} else if !d.locked && rand.Intn(10) == 0 {
		d.locked = true
		d.lockStart = now
		d.log.Info("Simulated lock regained")
	}
}

// updateSatellites simulates satellite constellation changes.
func (d *Dummy) updateSatellites() {
	for i := range d.satellites {
		// Update SNR
		d.satellites[i].SNR = 30 + rand.Float64()*15

		// Update elevation/azimuth slightly
		d.satellites[i].Elevation += rand.Intn(5) - 2
		if d.satellites[i].Elevation < 5 {
			d.satellites[i].Elevation = 5
		}
		if d.satellites[i].Elevation > 90 {
			d.satellites[i].Elevation = 90
		}

		d.satellites[i].Azimuth = (d.satellites[i].Azimuth + rand.Intn(3) - 1 + 360) % 360
	}
}

// Stop stops the dummy GPSDO.
func (d *Dummy) Stop() {
	d.mu.Lock()
	d.running = false
	d.mu.Unlock()

	d.log.Info("Dummy GPSDO stopped")
}

// GetTime returns the simulated GPS time.
func (d *Dummy) GetTime() (time.Time, error) {
	return time.Now(), nil
}

// GetOffset returns the simulated offset.
func (d *Dummy) GetOffset() (time.Duration, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.offset, nil
}

// IsLocked returns the simulated lock status.
func (d *Dummy) IsLocked() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.locked
}

// Priority returns the reference priority.
func (d *Dummy) Priority() int {
	return 0
}

// Quality returns the simulated reference quality.
func (d *Dummy) Quality() clock.ReferenceQuality {
	d.mu.RLock()
	defer d.mu.RUnlock()

	stratum := 1
	if !d.locked {
		stratum = 16
	}

	return clock.ReferenceQuality{
		Stratum:    stratum,
		Accuracy:   time.Duration(math.Abs(float64(d.offset))),
		LastUpdate: d.lastPPS,
	}
}

// GetStatus returns the simulated GPSDO status.
func (d *Dummy) GetStatus() Status {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var lockDuration time.Duration
	if !d.lockStart.IsZero() && d.locked {
		lockDuration = time.Since(d.lockStart)
	}

	// Calculate average SNR
	var avgSNR float64
	for _, sat := range d.satellites {
		avgSNR += sat.SNR
	}
	if len(d.satellites) > 0 {
		avgSNR /= float64(len(d.satellites))
	}

	return Status{
		Type:           "dummy",
		Connected:      d.running,
		Locked:         d.locked,
		LockDuration:   lockDuration,
		Satellites:     len(d.satellites),
		SignalStrength: avgSNR,
		Offset:         d.offset,
		Latitude:       37.7749,  // San Francisco
		Longitude:      -122.4194,
		Altitude:       10,
		HDOP:           1.2,
		PPSCount:       d.ppsCount,
		LastPPS:        d.lastPPS,
		LastNMEA:       d.lastPPS,
		Firmware:       "DUMMY-1.0",
		SerialNumber:   "DUMMY-000001",
	}
}

// GetSatellites returns simulated satellite information.
func (d *Dummy) GetSatellites() []Satellite {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]Satellite, len(d.satellites))
	copy(result, d.satellites)
	return result
}
