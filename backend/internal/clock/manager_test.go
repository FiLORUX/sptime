// Package clock tests for clock discipline and state management.
package clock

import (
	"testing"
	"time"

	"github.com/sptime/sptime/internal/logging"
	"github.com/sptime/sptime/internal/metrics"
)

// mockTimeReference implements TimeReference for testing.
type mockTimeReference struct {
	name     string
	locked   bool
	offset   time.Duration
	priority int
	stratum  int
}

func (m *mockTimeReference) Name() string                          { return m.name }
func (m *mockTimeReference) GetTime() (time.Time, error)           { return time.Now(), nil }
func (m *mockTimeReference) GetOffset() (time.Duration, error)     { return m.offset, nil }
func (m *mockTimeReference) IsLocked() bool                        { return m.locked }
func (m *mockTimeReference) Priority() int                         { return m.priority }
func (m *mockTimeReference) Quality() ReferenceQuality {
	return ReferenceQuality{
		Stratum:    m.stratum,
		LastUpdate: time.Now(),
	}
}

func TestManagerStateInit(t *testing.T) {
	log := logging.New("test")
	m := metrics.New()
	mgr := NewManager(log, m)

	if mgr.GetState() != StateInit {
		t.Errorf("Initial state should be INIT, got %v", mgr.GetState())
	}
}

func TestManagerStateTransitions(t *testing.T) {
	log := logging.New("test")
	m := metrics.New()
	mgr := NewManager(log, m)

	// Create a mock reference
	ref := &mockTimeReference{
		name:     "TestRef",
		locked:   true,
		offset:   10 * time.Microsecond,
		priority: 1,
		stratum:  1,
	}

	// Set primary reference
	mgr.SetPrimaryReference(ref)

	// Simulate ticks to trigger state transitions
	// After setting reference, tick should move to SYNCING
	mgr.tick()

	if mgr.GetState() != StateSyncing {
		t.Errorf("State should transition to SYNCING after reference acquired, got %v", mgr.GetState())
	}

	// Simulate several ticks with low offset to achieve LOCKED state
	for i := 0; i < 10; i++ {
		mgr.tick()
	}

	if mgr.GetState() != StateLocked {
		t.Errorf("State should transition to LOCKED after stable offset, got %v", mgr.GetState())
	}
}

func TestManagerReferenceLoss(t *testing.T) {
	log := logging.New("test")
	m := metrics.New()
	mgr := NewManager(log, m)

	ref := &mockTimeReference{
		name:     "TestRef",
		locked:   true,
		offset:   10 * time.Microsecond,
		priority: 1,
		stratum:  1,
	}

	mgr.SetPrimaryReference(ref)

	// Get to LOCKED state
	for i := 0; i < 10; i++ {
		mgr.tick()
	}

	// Simulate reference loss
	ref.locked = false

	mgr.tick()

	// Should transition to HOLDOVER
	if mgr.GetState() != StateHoldover {
		t.Errorf("State should transition to HOLDOVER after reference loss, got %v", mgr.GetState())
	}
}

func TestManagerGetStatus(t *testing.T) {
	log := logging.New("test")
	m := metrics.New()
	mgr := NewManager(log, m)

	ref := &mockTimeReference{
		name:     "GPS",
		locked:   true,
		offset:   50 * time.Nanosecond,
		priority: 0,
		stratum:  0,
	}

	mgr.SetPrimaryReference(ref)

	status := mgr.GetStatus()

	if status.State != StateInit {
		t.Errorf("Initial status state should be INIT, got %v", status.State)
	}

	if status.PrimaryReference != "GPS" {
		t.Errorf("Primary reference should be GPS, got %v", status.PrimaryReference)
	}
}

func TestManagerStratum(t *testing.T) {
	log := logging.New("test")
	m := metrics.New()
	mgr := NewManager(log, m)

	// Without reference, stratum should be 16 (unsynchronised)
	if mgr.GetStratum() != 16 {
		t.Errorf("Stratum without reference should be 16, got %d", mgr.GetStratum())
	}

	// With stratum 1 reference, should be stratum 2
	ref := &mockTimeReference{
		name:     "GPS",
		locked:   true,
		offset:   0,
		priority: 0,
		stratum:  1,
	}

	mgr.SetPrimaryReference(ref)
	mgr.tick() // Activate reference

	if mgr.GetStratum() != 2 {
		t.Errorf("Stratum with stratum-1 reference should be 2, got %d", mgr.GetStratum())
	}
}

func TestManagerMultipleReferences(t *testing.T) {
	log := logging.New("test")
	m := metrics.New()
	mgr := NewManager(log, m)

	primary := &mockTimeReference{
		name:     "GPS",
		locked:   true,
		offset:   10 * time.Nanosecond,
		priority: 0,
		stratum:  0,
	}

	secondary := &mockTimeReference{
		name:     "NTP",
		locked:   true,
		offset:   100 * time.Microsecond,
		priority: 10,
		stratum:  2,
	}

	mgr.SetPrimaryReference(primary)
	mgr.AddSecondaryReference(secondary)

	mgr.tick()

	// Should prefer primary (GPS) due to lower priority
	if mgr.GetActiveReference() != "GPS" {
		t.Errorf("Active reference should be GPS, got %v", mgr.GetActiveReference())
	}

	// Disable primary
	primary.locked = false
	mgr.tick()

	// Should fall back to secondary
	if mgr.GetActiveReference() != "NTP" {
		t.Errorf("Active reference should fall back to NTP, got %v", mgr.GetActiveReference())
	}
}

func TestManagerOffsetCalculation(t *testing.T) {
	log := logging.New("test")
	m := metrics.New()
	mgr := NewManager(log, m)

	ref := &mockTimeReference{
		name:     "GPS",
		locked:   true,
		offset:   100 * time.Nanosecond,
		priority: 0,
		stratum:  0,
	}

	mgr.SetPrimaryReference(ref)

	// Tick several times to populate samples
	for i := 0; i < 8; i++ {
		mgr.tick()
	}

	offset := mgr.GetOffset()

	// Offset should be close to the reference offset
	if offset < 50*time.Nanosecond || offset > 150*time.Nanosecond {
		t.Errorf("Offset should be around 100ns, got %v", offset)
	}
}
