// Package ntp provides NTP peer management for upstream time sources.
package ntp

import (
	"context"
	"fmt"
	"math"
	"net"
	"sync"
	"time"

	"github.com/sptime/sptime/internal/clock"
	"github.com/sptime/sptime/internal/config"
	"github.com/sptime/sptime/internal/logging"
)

// PeerStatus represents the current status of an NTP peer.
type PeerStatus struct {
	Address     string    `json:"address"`
	Stratum     int       `json:"stratum"`
	RefID       string    `json:"ref_id"`
	Reachable   bool      `json:"reachable"`
	Reach       uint8     `json:"reach"`
	Offset      float64   `json:"offset_us"`
	Delay       float64   `json:"delay_us"`
	Jitter      float64   `json:"jitter_us"`
	LastPoll    time.Time `json:"last_poll"`
	NextPoll    time.Time `json:"next_poll"`
	PollCount   uint64    `json:"poll_count"`
	ErrorCount  uint64    `json:"error_count"`
	Selected    bool      `json:"selected"`
	NTSEnabled  bool      `json:"nts_enabled"`
}

// Peer represents an upstream NTP peer.
type Peer struct {
	mu sync.RWMutex

	cfg      config.PeerConfig
	server   *Server
	log      *logging.Logger

	addr     *net.UDPAddr
	conn     *net.UDPConn
	running  bool

	// Peer state
	stratum     uint8
	refID       [4]byte
	reach       uint8    // Reachability register (8-bit shift register)
	offset      float64  // Offset in microseconds
	delay       float64  // Round-trip delay in microseconds
	dispersion  float64  // Dispersion in microseconds
	jitter      float64  // Jitter (RMS of offset differences)
	lastPoll    time.Time
	pollCount   uint64
	errorCount  uint64

	// Filter for offset calculations
	offsets     [8]float64
	delays      [8]float64
	filterIdx   int
	filterCount int
}

// NewPeer creates a new NTP peer.
func NewPeer(cfg config.PeerConfig, server *Server, log *logging.Logger) (*Peer, error) {
	addr, err := net.ResolveUDPAddr("udp", cfg.Address)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve peer address: %w", err)
	}

	return &Peer{
		cfg:    cfg,
		server: server,
		log:    log.WithField("peer", cfg.Address),
		addr:   addr,
	}, nil
}

// Run starts the peer polling loop.
func (p *Peer) Run(ctx context.Context) {
	var err error
	p.conn, err = net.DialUDP("udp", nil, p.addr)
	if err != nil {
		p.log.Error("Failed to connect to peer", "error", err)
		return
	}
	defer p.conn.Close()

	p.mu.Lock()
	p.running = true
	p.mu.Unlock()

	// Initial burst if configured
	if p.cfg.IBurst {
		p.burst(ctx, 4)
	}

	// Main polling loop
	pollInterval := 1 << p.cfg.MinPoll
	if pollInterval < 16 {
		pollInterval = 16
	}

	ticker := time.NewTicker(time.Duration(pollInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll()
		}
	}
}

// burst sends multiple rapid polls.
func (p *Peer) burst(ctx context.Context, count int) {
	for i := 0; i < count; i++ {
		select {
		case <-ctx.Done():
			return
		default:
		}
		p.poll()
		time.Sleep(2 * time.Second)
	}
}

// poll sends a single NTP request to the peer.
func (p *Peer) poll() {
	p.mu.Lock()
	p.pollCount++
	p.mu.Unlock()

	txTime := time.Now()

	// Build request packet
	req := &Packet{
		LiVnMode:    uint8(LeapNoWarning<<6) | uint8(NTPv4<<3) | ModeClient,
		Stratum:     0,
		Poll:        int8(p.cfg.MinPoll),
		Precision:   -20,
		TxTimestamp: timeToNTP(txTime),
	}

	// Send request
	if _, err := p.conn.Write(serializePacket(req)); err != nil {
		p.log.Debug("Failed to send request", "error", err)
		p.recordError()
		return
	}

	// Set read timeout
	p.conn.SetReadDeadline(time.Now().Add(2 * time.Second))

	// Receive response
	buf := make([]byte, 512)
	n, err := p.conn.Read(buf)
	if err != nil {
		p.log.Debug("Failed to receive response", "error", err)
		p.recordError()
		return
	}

	rxTime := time.Now()

	// Parse response
	resp, err := parsePacket(buf[:n])
	if err != nil {
		p.log.Debug("Failed to parse response", "error", err)
		p.recordError()
		return
	}

	// Validate response
	if resp.Stratum == 0 || resp.Stratum > 15 {
		p.log.Debug("Invalid stratum in response", "stratum", resp.Stratum)
		p.recordError()
		return
	}

	// Calculate offset and delay
	t1 := txTime                       // Client transmit
	t2 := ntpToTime(resp.RxTimestamp)  // Server receive
	t3 := ntpToTime(resp.TxTimestamp)  // Server transmit
	t4 := rxTime                       // Client receive

	// Offset = ((t2 - t1) + (t3 - t4)) / 2
	offset := ((t2.Sub(t1) + t3.Sub(t4)) / 2).Seconds() * 1e6 // microseconds

	// Delay = (t4 - t1) - (t3 - t2)
	delay := (t4.Sub(t1) - t3.Sub(t2)).Seconds() * 1e6 // microseconds

	// Update peer state
	p.mu.Lock()
	p.stratum = resp.Stratum
	copy(p.refID[:], resp.ReferenceID[:])
	p.lastPoll = time.Now()

	// Update reachability register
	p.reach = (p.reach << 1) | 1

	// Add to filter
	p.offsets[p.filterIdx] = offset
	p.delays[p.filterIdx] = delay
	p.filterIdx = (p.filterIdx + 1) % 8
	if p.filterCount < 8 {
		p.filterCount++
	}

	// Calculate filtered values
	p.offset = p.calculateFilteredOffset()
	p.delay = p.calculateMinDelay()
	p.jitter = p.calculateJitter()
	p.mu.Unlock()

	p.log.Debug("Poll successful", "offset_us", offset, "delay_us", delay)

	// Notify server of update
	p.server.UpdateFromPeers()
}

// recordError updates state for a failed poll.
func (p *Peer) recordError() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.errorCount++
	p.reach = p.reach << 1 // Shift in a zero
}

// calculateFilteredOffset returns the median offset.
func (p *Peer) calculateFilteredOffset() float64 {
	if p.filterCount == 0 {
		return 0
	}

	// Create sorted copy
	sorted := make([]float64, p.filterCount)
	copy(sorted, p.offsets[:p.filterCount])

	// Sort using simple insertion sort
	for i := 1; i < len(sorted); i++ {
		key := sorted[i]
		j := i - 1
		for j >= 0 && sorted[j] > key {
			sorted[j+1] = sorted[j]
			j--
		}
		sorted[j+1] = key
	}

	// Return median
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

// calculateMinDelay returns the minimum delay.
func (p *Peer) calculateMinDelay() float64 {
	if p.filterCount == 0 {
		return 0
	}

	min := p.delays[0]
	for i := 1; i < p.filterCount; i++ {
		if p.delays[i] < min {
			min = p.delays[i]
		}
	}
	return min
}

// calculateJitter returns the RMS jitter.
func (p *Peer) calculateJitter() float64 {
	if p.filterCount < 2 {
		return 0
	}

	var sum float64
	for i := 1; i < p.filterCount; i++ {
		diff := p.offsets[i] - p.offsets[i-1]
		sum += diff * diff
	}

	return math.Sqrt(sum / float64(p.filterCount-1))
}

// Stop stops the peer.
func (p *Peer) Stop() {
	p.mu.Lock()
	p.running = false
	p.mu.Unlock()

	if p.conn != nil {
		p.conn.Close()
	}
}

// GetStatus returns the current peer status.
func (p *Peer) GetStatus() PeerStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return PeerStatus{
		Address:    p.cfg.Address,
		Stratum:    int(p.stratum),
		RefID:      string(p.refID[:]),
		Reachable:  p.reach != 0,
		Reach:      p.reach,
		Offset:     p.offset,
		Delay:      p.delay,
		Jitter:     p.jitter,
		LastPoll:   p.lastPoll,
		PollCount:  p.pollCount,
		ErrorCount: p.errorCount,
		NTSEnabled: p.cfg.NTSEnabled,
	}
}

// IsReachable returns true if the peer is reachable.
func (p *Peer) IsReachable() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.reach != 0
}

// GetOffsetMicroseconds returns the current offset in microseconds (for status display).
func (p *Peer) GetOffsetMicroseconds() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.offset
}

// Implements clock.TimeReference interface

// Name returns the peer name.
func (p *Peer) Name() string {
	return fmt.Sprintf("NTP:%s", p.cfg.Address)
}

// GetTime returns the estimated time from this peer.
func (p *Peer) GetTime() (time.Time, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return time.Now().Add(time.Duration(p.offset) * time.Microsecond), nil
}

// GetOffset returns the offset as a duration (implements clock.TimeReference).
func (p *Peer) GetOffset() (time.Duration, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return time.Duration(p.offset) * time.Microsecond, nil
}

// IsLocked returns true if the peer is reachable.
func (p *Peer) IsLocked() bool {
	return p.IsReachable()
}

// Priority returns the peer priority.
func (p *Peer) Priority() int {
	if p.cfg.Prefer {
		return 1
	}
	return int(p.stratum) + 10
}

// Quality returns the peer quality.
func (p *Peer) Quality() clock.ReferenceQuality {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return clock.ReferenceQuality{
		Stratum:      int(p.stratum),
		Accuracy:     time.Duration(p.jitter) * time.Microsecond,
		LastUpdate:   p.lastPoll,
		Reachability: p.reach,
	}
}
