// Package clock provides clock discipline and state management for SPTime.
// It implements a state machine for tracking synchronisation status and
// manages the relationship between time references (GPSDO, NTP, PTP).
package clock

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/sptime/sptime/internal/logging"
	"github.com/sptime/sptime/internal/metrics"
)

// State represents the current clock synchronisation state.
type State string

const (
	StateInit     State = "INIT"
	StateSyncing  State = "SYNCING"
	StateLocked   State = "LOCKED"
	StateHoldover State = "HOLDOVER"
	StateFault    State = "FAULT"
)

// TimeReference is the interface that time sources must implement.
type TimeReference interface {
	Name() string
	GetTime() (time.Time, error)
	GetOffset() (time.Duration, error)
	IsLocked() bool
	Priority() int
	Quality() ReferenceQuality
}

// ReferenceQuality describes the quality of a time reference.
type ReferenceQuality struct {
	Stratum       int           `json:"stratum"`
	Accuracy      time.Duration `json:"accuracy_ns"`
	Stability     float64       `json:"stability_ppb"`
	LastUpdate    time.Time     `json:"last_update"`
	Reachability  uint8         `json:"reachability"`
}

// Status represents the current clock status.
type Status struct {
	State             State          `json:"state"`
	CurrentTime       time.Time      `json:"current_time"`
	Offset            time.Duration  `json:"offset_ns"`
	OffsetStdDev      time.Duration  `json:"offset_stddev_ns"`
	Drift             float64        `json:"drift_ppb"`
	PrimaryReference  string         `json:"primary_reference"`
	ActiveReference   string         `json:"active_reference"`
	Stratum           int            `json:"stratum"`
	LastSync          time.Time      `json:"last_sync"`
	HoldoverStarted   *time.Time     `json:"holdover_started,omitempty"`
	HoldoverRemaining time.Duration  `json:"holdover_remaining_ns,omitempty"`
}

// Manager handles clock discipline and state transitions.
type Manager struct {
	mu sync.RWMutex

	state            State
	primaryRef       TimeReference
	secondaryRefs    []TimeReference
	activeRef        TimeReference

	// Clock discipline data
	offset           time.Duration
	offsetSamples    []time.Duration
	drift            float64
	driftSamples     []float64
	lastSync         time.Time
	holdoverStart    *time.Time

	// Configuration
	holdoverTimeout  time.Duration
	lockThreshold    time.Duration
	faultThreshold   time.Duration
	sampleWindow     int

	log     *logging.Logger
	metrics *metrics.Collector
}

// NewManager creates a new clock manager.
func NewManager(log *logging.Logger, m *metrics.Collector) *Manager {
	mgr := &Manager{
		state:           StateInit,
		holdoverTimeout: 4 * time.Hour,
		lockThreshold:   100 * time.Microsecond,
		faultThreshold:  1 * time.Second,
		sampleWindow:    8,
		offsetSamples:   make([]time.Duration, 0, 8),
		driftSamples:    make([]float64, 0, 8),
		log:             log,
		metrics:         m,
	}

	m.SetClockState(string(StateInit))
	return mgr
}

// SetPrimaryReference sets the primary time reference (typically GPSDO).
func (m *Manager) SetPrimaryReference(ref TimeReference) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.primaryRef = ref
	m.log.Info("Primary reference set", "name", ref.Name(), "priority", ref.Priority())
}

// AddSecondaryReference adds a secondary time reference (e.g., NTP peer).
func (m *Manager) AddSecondaryReference(ref TimeReference) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.secondaryRefs = append(m.secondaryRefs, ref)
	m.log.Info("Secondary reference added", "name", ref.Name(), "priority", ref.Priority())
}

// GetStatus returns the current clock status.
func (m *Manager) GetStatus() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := Status{
		State:       m.state,
		CurrentTime: time.Now(),
		Offset:      m.offset,
		Drift:       m.drift,
		LastSync:    m.lastSync,
		Stratum:     m.calculateStratum(),
	}

	if m.primaryRef != nil {
		status.PrimaryReference = m.primaryRef.Name()
	}
	if m.activeRef != nil {
		status.ActiveReference = m.activeRef.Name()
	}
	if m.holdoverStart != nil {
		status.HoldoverStarted = m.holdoverStart
		elapsed := time.Since(*m.holdoverStart)
		status.HoldoverRemaining = m.holdoverTimeout - elapsed
	}

	// Calculate offset standard deviation
	if len(m.offsetSamples) > 1 {
		status.OffsetStdDev = m.calculateStdDev()
	}

	return status
}

// calculateStratum determines the current stratum level.
func (m *Manager) calculateStratum() int {
	if m.activeRef == nil {
		return 16 // Unsynchronized
	}
	quality := m.activeRef.Quality()
	return quality.Stratum + 1
}

// calculateStdDev calculates the standard deviation of offset samples.
func (m *Manager) calculateStdDev() time.Duration {
	if len(m.offsetSamples) < 2 {
		return 0
	}

	var sum float64
	for _, s := range m.offsetSamples {
		sum += float64(s)
	}
	mean := sum / float64(len(m.offsetSamples))

	var variance float64
	for _, s := range m.offsetSamples {
		diff := float64(s) - mean
		variance += diff * diff
	}
	variance /= float64(len(m.offsetSamples) - 1)

	return time.Duration(math.Sqrt(variance))
}

// Run starts the clock discipline loop.
func (m *Manager) Run(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	m.log.Info("Clock discipline loop started")

	for {
		select {
		case <-ctx.Done():
			m.log.Info("Clock discipline loop stopped")
			return
		case <-ticker.C:
			m.tick()
		}
	}
}

// tick performs one iteration of the clock discipline loop.
func (m *Manager) tick() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Select the best available reference
	newRef := m.selectBestReference()

	// Handle reference changes
	if newRef != m.activeRef {
		if newRef == nil {
			m.handleReferenceLost()
		} else {
			m.handleReferenceFound(newRef)
		}
	}

	// Update clock based on active reference
	if m.activeRef != nil {
		m.updateClock()
	}

	// Update state based on current conditions
	m.updateState()

	// Update metrics
	m.metrics.SetClockState(string(m.state))
	m.metrics.ClockOffset.Set(float64(m.offset))
	m.metrics.ClockDrift.Set(m.drift)
}

// selectBestReference selects the best available time reference.
func (m *Manager) selectBestReference() TimeReference {
	// Primary reference has highest priority if locked
	if m.primaryRef != nil && m.primaryRef.IsLocked() {
		return m.primaryRef
	}

	// Find the best secondary reference
	var best TimeReference
	var bestPriority int = 256

	for _, ref := range m.secondaryRefs {
		if ref.IsLocked() && ref.Priority() < bestPriority {
			best = ref
			bestPriority = ref.Priority()
		}
	}

	return best
}

// handleReferenceLost handles loss of the active reference.
func (m *Manager) handleReferenceLost() {
	m.log.Warn("Active reference lost", "previous", m.activeRef.Name())
	m.activeRef = nil

	if m.state == StateLocked {
		now := time.Now()
		m.holdoverStart = &now
		m.log.Info("Entering holdover mode")
	}
}

// handleReferenceFound handles acquisition of a new reference.
func (m *Manager) handleReferenceFound(ref TimeReference) {
	if m.activeRef != nil {
		m.log.Info("Switching reference", "from", m.activeRef.Name(), "to", ref.Name())
	} else {
		m.log.Info("Reference acquired", "name", ref.Name())
	}
	m.activeRef = ref
	m.holdoverStart = nil
}

// updateClock updates clock offset and drift from the active reference.
func (m *Manager) updateClock() {
	offset, err := m.activeRef.GetOffset()
	if err != nil {
		m.log.Warn("Failed to get offset from reference", "error", err)
		return
	}

	// Update offset samples (sliding window)
	m.offsetSamples = append(m.offsetSamples, offset)
	if len(m.offsetSamples) > m.sampleWindow {
		m.offsetSamples = m.offsetSamples[1:]
	}

	// Calculate filtered offset (median filter for robustness)
	m.offset = m.medianOffset()

	// Calculate drift if we have enough samples
	if len(m.offsetSamples) >= 2 {
		m.updateDrift()
	}

	m.lastSync = time.Now()
	m.metrics.ClockLastUpdate.Set(float64(m.lastSync.Unix()))
}

// medianOffset returns the median of offset samples.
func (m *Manager) medianOffset() time.Duration {
	if len(m.offsetSamples) == 0 {
		return 0
	}

	// Create a copy and sort
	sorted := make([]time.Duration, len(m.offsetSamples))
	copy(sorted, m.offsetSamples)

	// Simple bubble sort for small arrays
	for i := 0; i < len(sorted)-1; i++ {
		for j := 0; j < len(sorted)-i-1; j++ {
			if sorted[j] > sorted[j+1] {
				sorted[j], sorted[j+1] = sorted[j+1], sorted[j]
			}
		}
	}

	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

// updateDrift calculates clock drift from offset samples.
func (m *Manager) updateDrift() {
	if len(m.offsetSamples) < 2 {
		return
	}

	// Simple linear regression to estimate drift
	// drift = (offset_n - offset_0) / time_interval
	first := m.offsetSamples[0]
	last := m.offsetSamples[len(m.offsetSamples)-1]
	interval := float64(len(m.offsetSamples)) // Assuming 1 second samples

	driftPerSec := float64(last-first) / interval
	m.drift = driftPerSec / 1e9 * 1e9 // Convert to PPB
}

// updateState updates the clock state based on current conditions.
func (m *Manager) updateState() {
	absOffset := m.offset
	if absOffset < 0 {
		absOffset = -absOffset
	}

	prevState := m.state

	switch m.state {
	case StateInit:
		if m.activeRef != nil {
			m.state = StateSyncing
		}

	case StateSyncing:
		if m.activeRef == nil {
			m.state = StateFault
		} else if absOffset < m.lockThreshold && len(m.offsetSamples) >= m.sampleWindow/2 {
			m.state = StateLocked
		}

	case StateLocked:
		if m.activeRef == nil {
			m.state = StateHoldover
		} else if absOffset > m.faultThreshold {
			m.state = StateFault
		}

	case StateHoldover:
		if m.activeRef != nil {
			m.state = StateSyncing
		} else if m.holdoverStart != nil && time.Since(*m.holdoverStart) > m.holdoverTimeout {
			m.state = StateFault
		}

	case StateFault:
		if m.activeRef != nil {
			m.state = StateSyncing
			// Reset samples on recovery
			m.offsetSamples = m.offsetSamples[:0]
			m.driftSamples = m.driftSamples[:0]
		}
	}

	if m.state != prevState {
		m.log.Info("Clock state changed", "from", prevState, "to", m.state)
	}
}

// GetTime returns the disciplined time.
func (m *Manager) GetTime() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return time.Now().Add(-m.offset)
}

// GetOffset returns the current clock offset.
func (m *Manager) GetOffset() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.offset
}

// GetState returns the current clock state.
func (m *Manager) GetState() State {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// GetStratum returns the current stratum level.
func (m *Manager) GetStratum() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.calculateStratum()
}

// IsLocked returns true if the clock is in locked state.
func (m *Manager) IsLocked() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state == StateLocked
}

// GetActiveReference returns the name of the active reference.
func (m *Manager) GetActiveReference() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.activeRef == nil {
		return ""
	}
	return m.activeRef.Name()
}
