// Package gpsdo provides Network implementation for network-based GPS time sources.
// Supports gpsd JSON protocol for interfacing with GPS daemons over the network.
package gpsdo

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/sptime/sptime/internal/clock"
	"github.com/sptime/sptime/internal/config"
	"github.com/sptime/sptime/internal/logging"
)

// gpsdTPV represents a gpsd TPV (Time-Position-Velocity) message.
type gpsdTPV struct {
	Class  string  `json:"class"`
	Mode   int     `json:"mode"`
	Time   string  `json:"time"`
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
	Alt    float64 `json:"alt"`
}

// gpsdSKY represents a gpsd SKY (satellite info) message.
type gpsdSKY struct {
	Class      string `json:"class"`
	NSat       int    `json:"nSat"`
	USat       int    `json:"uSat"`
}

// Network implements TimeReference for network-based GPS time sources.
// Supports gpsd JSON protocol (https://gpsd.gitlab.io/gpsd/gpsd_json.html).
type Network struct {
	mu sync.RWMutex

	cfg config.GPSDOConfig
	log *logging.Logger

	running   bool
	connected bool
	locked    bool
	lockStart time.Time

	// Network connection state
	address  string
	conn     net.Conn
	lastData time.Time
	lastErr  string

	// GPS data
	gpsTime    time.Time
	offset     time.Duration
	satellites int
	mode       int // GPS mode: 0=unknown, 1=no fix, 2=2D, 3=3D
}

// NewNetwork creates a new Network GPSDO instance.
func NewNetwork(cfg config.GPSDOConfig, log *logging.Logger) (*Network, error) {
	if cfg.NetworkAddr == "" {
		return nil, fmt.Errorf("network address is required for network GPSDO")
	}

	return &Network{
		cfg:     cfg,
		log:     log.WithField("gpsdo", "network"),
		address: cfg.NetworkAddr,
	}, nil
}

// Name returns the reference name.
func (n *Network) Name() string {
	return fmt.Sprintf("GPSDO:Network:%s", n.address)
}

// Start starts the network GPSDO client.
func (n *Network) Start(ctx context.Context) error {
	n.mu.Lock()
	n.running = true
	n.mu.Unlock()

	n.log.Info("Network GPSDO started", "address", n.address)

	// Main loop with reconnection
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		if err := n.connect(); err != nil {
			n.log.Debug("Failed to connect to gpsd", "error", err)
			n.mu.Lock()
			n.lastErr = err.Error()
			n.connected = false
			n.locked = false
			n.mu.Unlock()

			// Wait before retry
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(5 * time.Second):
				continue
			}
		}

		// Read data from gpsd
		n.readLoop(ctx)
	}
}

// connect establishes connection to gpsd.
func (n *Network) connect() error {
	conn, err := net.DialTimeout("tcp", n.address, 5*time.Second)
	if err != nil {
		return fmt.Errorf("dial failed: %w", err)
	}

	n.mu.Lock()
	n.conn = conn
	n.connected = true
	n.lastErr = ""
	n.mu.Unlock()

	// Enable WATCH mode for gpsd to stream data
	_, err = conn.Write([]byte(`?WATCH={"enable":true,"json":true}`))
	if err != nil {
		conn.Close()
		return fmt.Errorf("failed to enable watch: %w", err)
	}

	n.log.Info("Connected to gpsd", "address", n.address)
	return nil
}

// readLoop reads and processes gpsd JSON messages.
func (n *Network) readLoop(ctx context.Context) {
	n.mu.RLock()
	conn := n.conn
	n.mu.RUnlock()

	if conn == nil {
		return
	}

	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 4096), 4096)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line := scanner.Bytes()
		n.processMessage(line)
	}

	if err := scanner.Err(); err != nil {
		n.log.Debug("Read error", "error", err)
	}

	// Connection lost
	n.mu.Lock()
	n.connected = false
	n.locked = false
	if n.conn != nil {
		n.conn.Close()
		n.conn = nil
	}
	n.mu.Unlock()
}

// processMessage parses and handles a gpsd JSON message.
func (n *Network) processMessage(data []byte) {
	// Determine message class
	var base struct {
		Class string `json:"class"`
	}
	if err := json.Unmarshal(data, &base); err != nil {
		return
	}

	switch base.Class {
	case "TPV":
		var tpv gpsdTPV
		if err := json.Unmarshal(data, &tpv); err != nil {
			return
		}
		n.handleTPV(&tpv)

	case "SKY":
		var sky gpsdSKY
		if err := json.Unmarshal(data, &sky); err != nil {
			return
		}
		n.handleSKY(&sky)
	}
}

// handleTPV processes a TPV message.
func (n *Network) handleTPV(tpv *gpsdTPV) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.mode = tpv.Mode
	n.lastData = time.Now()

	// Parse GPS time if available
	if tpv.Time != "" {
		gpsTime, err := time.Parse(time.RFC3339Nano, tpv.Time)
		if err == nil {
			n.gpsTime = gpsTime
			n.offset = time.Now().Sub(gpsTime)

			// Mode 2 or 3 means we have a fix
			wasLocked := n.locked
			n.locked = tpv.Mode >= 2

			if n.locked && !wasLocked {
				n.lockStart = time.Now()
				n.log.Info("GPS lock acquired", "mode", tpv.Mode)
			} else if !n.locked && wasLocked {
				n.log.Warn("GPS lock lost")
			}
		}
	}
}

// handleSKY processes a SKY message.
func (n *Network) handleSKY(sky *gpsdSKY) {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.satellites = sky.USat // Used satellites
	if sky.USat == 0 {
		n.satellites = sky.NSat // Fall back to visible satellites
	}
}

// Stop stops the network GPSDO client.
func (n *Network) Stop() {
	n.mu.Lock()
	n.running = false
	n.connected = false
	if n.conn != nil {
		n.conn.Close()
		n.conn = nil
	}
	n.mu.Unlock()

	n.log.Info("Network GPSDO stopped")
}

// GetTime returns the GPS time from the network source.
func (n *Network) GetTime() (time.Time, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if !n.locked {
		return time.Time{}, fmt.Errorf("network GPS not locked")
	}

	return n.gpsTime, nil
}

// GetOffset returns the offset between GPS time and system time.
func (n *Network) GetOffset() (time.Duration, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if !n.locked {
		return 0, fmt.Errorf("network GPS not locked")
	}

	return n.offset, nil
}

// IsLocked returns true if the network GPS has a valid fix.
func (n *Network) IsLocked() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.locked && n.connected
}

// Priority returns the reference priority.
func (n *Network) Priority() int {
	return 1 // Slightly lower priority than local GPS
}

// Quality returns the reference quality information.
func (n *Network) Quality() clock.ReferenceQuality {
	n.mu.RLock()
	defer n.mu.RUnlock()

	stratum := 1
	if !n.locked {
		stratum = 16
	}

	return clock.ReferenceQuality{
		Stratum:    stratum,
		Accuracy:   10 * time.Millisecond, // Network adds latency
		LastUpdate: n.lastData,
	}
}

// GetStatus returns the current GPSDO status.
func (n *Network) GetStatus() Status {
	n.mu.RLock()
	defer n.mu.RUnlock()

	var lockDuration time.Duration
	if !n.lockStart.IsZero() && n.locked {
		lockDuration = time.Since(n.lockStart)
	}

	return Status{
		Type:         "network",
		Connected:    n.connected,
		Locked:       n.locked,
		LockDuration: lockDuration,
		Satellites:   n.satellites,
		Offset:       n.offset,
		Error:        n.lastErr,
	}
}

// GetSatellites returns satellite information (if available from network source).
func (n *Network) GetSatellites() []Satellite {
	// Network sources may not provide detailed satellite info
	return nil
}
