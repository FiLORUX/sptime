// Package ptp implements IEEE 1588 Precision Time Protocol (PTPv2) Grandmaster.
// Supports default profile with architecture for SMPTE 2059 and telecom profiles.
package ptp

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/sptime/sptime/internal/clock"
	"github.com/sptime/sptime/internal/config"
	"github.com/sptime/sptime/internal/logging"
	"github.com/sptime/sptime/internal/metrics"
)

const (
	// PTP message types
	MsgSync            = 0x0
	MsgDelayReq        = 0x1
	MsgPDelayReq       = 0x2
	MsgPDelayResp      = 0x3
	MsgFollowUp        = 0x8
	MsgDelayResp       = 0x9
	MsgPDelayRespFU    = 0xA
	MsgAnnounce        = 0xB
	MsgSignaling       = 0xC
	MsgManagement      = 0xD

	// PTP ports (multicast)
	EventPort   = 319
	GeneralPort = 320

	// Multicast addresses
	DefaultMulticast = "224.0.1.129"
	PeerMulticast    = "224.0.0.107"

	// PTP header size
	HeaderSize = 34

	// Clock identity size
	ClockIDSize = 8
)

// Profile represents a PTP profile.
type Profile string

const (
	ProfileDefault  Profile = "default"
	ProfileSMPTE    Profile = "smpte2059"
	ProfileTelecom  Profile = "telecom"
	ProfilePower    Profile = "power"
)

// PortState represents the PTP port state.
type PortState int

const (
	StateInitializing PortState = iota
	StateFaulty
	StateDisabled
	StateListening
	StatePreMaster
	StateMaster
	StatePassive
	StateUncalibrated
	StateSlave
)

func (s PortState) String() string {
	names := []string{
		"INITIALIZING", "FAULTY", "DISABLED", "LISTENING",
		"PRE_MASTER", "MASTER", "PASSIVE", "UNCALIBRATED", "SLAVE",
	}
	if int(s) < len(names) {
		return names[s]
	}
	return "UNKNOWN"
}

// Grandmaster implements a PTP Grandmaster clock.
type Grandmaster struct {
	mu sync.RWMutex

	cfg     config.PTPConfig
	clock   *clock.Manager
	log     *logging.Logger
	metrics *metrics.Collector

	// Network
	eventConn   *net.UDPConn
	generalConn *net.UDPConn
	iface       *net.Interface

	// State
	running      bool
	portState    PortState
	clockID      [ClockIDSize]byte
	sequenceID   uint16

	// Clock properties
	priority1     uint8
	priority2     uint8
	clockClass    uint8
	clockAccuracy uint8
	variance      uint16

	// Timing
	announceInterval time.Duration
	syncInterval     time.Duration
	lastAnnounce     time.Time
	lastSync         time.Time

	// Stats
	syncCount     uint64
	announceCount uint64
	delayReqCount uint64
}

// Status represents PTP Grandmaster status.
type Status struct {
	Running         bool      `json:"running"`
	PortState       string    `json:"port_state"`
	ClockID         string    `json:"clock_id"`
	Domain          int       `json:"domain"`
	Priority1       int       `json:"priority1"`
	Priority2       int       `json:"priority2"`
	ClockClass      int       `json:"clock_class"`
	ClockAccuracy   int       `json:"clock_accuracy"`
	Profile         string    `json:"profile"`
	Interface       string    `json:"interface"`
	SyncCount       uint64    `json:"sync_count"`
	AnnounceCount   uint64    `json:"announce_count"`
	DelayReqCount   uint64    `json:"delay_req_count"`
	Offset          float64   `json:"offset_ns"`
	PathDelay       float64   `json:"path_delay_ns"`
}

// NewGrandmaster creates a new PTP Grandmaster.
func NewGrandmaster(cfg config.PTPConfig, clockMgr *clock.Manager, log *logging.Logger, m *metrics.Collector) (*Grandmaster, error) {
	// Get network interface
	iface, err := net.InterfaceByName(cfg.Interface)
	if err != nil {
		return nil, fmt.Errorf("failed to get interface %s: %w", cfg.Interface, err)
	}

	gm := &Grandmaster{
		cfg:       cfg,
		clock:     clockMgr,
		log:       log.WithField("module", "ptp"),
		metrics:   m,
		iface:     iface,
		portState: StateInitializing,

		priority1:     uint8(cfg.Priority1),
		priority2:     uint8(cfg.Priority2),
		clockClass:    uint8(cfg.ClockClass),
		clockAccuracy: uint8(cfg.ClockAccuracy),
		variance:      0xFFFF,
	}

	// Generate clock identity from MAC address
	gm.generateClockID()

	// Calculate intervals from log values
	gm.announceInterval = time.Duration(1<<uint(cfg.LogAnnounceInt)) * time.Second
	gm.syncInterval = time.Duration(1<<uint(cfg.LogSyncInt)) * time.Second

	if gm.announceInterval < time.Second {
		gm.announceInterval = time.Second
	}
	if gm.syncInterval < 125*time.Millisecond {
		gm.syncInterval = 125 * time.Millisecond
	}

	return gm, nil
}

// generateClockID generates a clock identity from the MAC address.
func (gm *Grandmaster) generateClockID() {
	addrs, err := gm.iface.Addrs()
	if err != nil || len(addrs) == 0 {
		// Use random ID if no MAC available
		binary.BigEndian.PutUint64(gm.clockID[:], uint64(time.Now().UnixNano()))
		return
	}

	// Use MAC address with EUI-64 conversion
	mac := gm.iface.HardwareAddr
	if len(mac) >= 6 {
		gm.clockID[0] = mac[0] ^ 0x02 // Flip local/universal bit
		gm.clockID[1] = mac[1]
		gm.clockID[2] = mac[2]
		gm.clockID[3] = 0xFF
		gm.clockID[4] = 0xFE
		gm.clockID[5] = mac[3]
		gm.clockID[6] = mac[4]
		gm.clockID[7] = mac[5]
	}
}

// Start starts the PTP Grandmaster.
func (gm *Grandmaster) Start(ctx context.Context) error {
	// Join multicast groups
	eventAddr := &net.UDPAddr{
		IP:   net.ParseIP(DefaultMulticast),
		Port: EventPort,
	}
	generalAddr := &net.UDPAddr{
		IP:   net.ParseIP(DefaultMulticast),
		Port: GeneralPort,
	}

	var err error
	gm.eventConn, err = net.ListenMulticastUDP("udp4", gm.iface, eventAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on event port: %w", err)
	}

	gm.generalConn, err = net.ListenMulticastUDP("udp4", gm.iface, generalAddr)
	if err != nil {
		gm.eventConn.Close()
		return fmt.Errorf("failed to listen on general port: %w", err)
	}

	gm.mu.Lock()
	gm.running = true
	gm.portState = StateMaster
	gm.mu.Unlock()

	gm.log.Info("PTP Grandmaster started",
		"interface", gm.cfg.Interface,
		"domain", gm.cfg.Domain,
		"clock_id", fmt.Sprintf("%x", gm.clockID))

	// Start message handlers
	go gm.handleEventMessages(ctx)
	go gm.handleGeneralMessages(ctx)

	// Start transmit loops
	go gm.announceLoop(ctx)
	go gm.syncLoop(ctx)

	<-ctx.Done()
	return nil
}

// announceLoop sends periodic Announce messages.
func (gm *Grandmaster) announceLoop(ctx context.Context) {
	ticker := time.NewTicker(gm.announceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			gm.sendAnnounce()
		}
	}
}

// syncLoop sends periodic Sync messages.
func (gm *Grandmaster) syncLoop(ctx context.Context) {
	ticker := time.NewTicker(gm.syncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			gm.sendSync()
		}
	}
}

// handleEventMessages handles incoming event messages.
func (gm *Grandmaster) handleEventMessages(ctx context.Context) {
	buf := make([]byte, 1500)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		gm.eventConn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, addr, err := gm.eventConn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			gm.log.Error("Event read error", "error", err)
			continue
		}

		if n < HeaderSize {
			continue
		}

		rxTime := time.Now()
		gm.handleMessage(buf[:n], addr, rxTime)
	}
}

// handleGeneralMessages handles incoming general messages.
func (gm *Grandmaster) handleGeneralMessages(ctx context.Context) {
	buf := make([]byte, 1500)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		gm.generalConn.SetReadDeadline(time.Now().Add(1 * time.Second))
		n, addr, err := gm.generalConn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			gm.log.Error("General read error", "error", err)
			continue
		}

		if n < HeaderSize {
			continue
		}

		gm.handleMessage(buf[:n], addr, time.Now())
	}
}

// handleMessage processes a received PTP message.
func (gm *Grandmaster) handleMessage(data []byte, addr *net.UDPAddr, rxTime time.Time) {
	msgType := data[0] & 0x0F
	domain := data[4]

	// Check domain
	if int(domain) != gm.cfg.Domain {
		return
	}

	switch msgType {
	case MsgDelayReq:
		gm.handleDelayReq(data, addr, rxTime)
	case MsgPDelayReq:
		gm.handlePDelayReq(data, addr, rxTime)
	case MsgAnnounce:
		// As Grandmaster, we can use announce messages for BMCA
		gm.log.Debug("Received Announce", "from", addr)
	}
}

// handleDelayReq handles Delay_Req messages.
func (gm *Grandmaster) handleDelayReq(data []byte, addr *net.UDPAddr, rxTime time.Time) {
	gm.mu.Lock()
	gm.delayReqCount++
	gm.mu.Unlock()

	gm.metrics.PTPDelayReq.Inc()

	// Extract sequence ID and source port identity
	seqID := binary.BigEndian.Uint16(data[30:32])
	var sourceID [10]byte
	copy(sourceID[:], data[20:30])

	// Send Delay_Resp
	gm.sendDelayResp(addr, seqID, sourceID, rxTime)
}

// handlePDelayReq handles Peer_Delay_Req messages.
func (gm *Grandmaster) handlePDelayReq(data []byte, addr *net.UDPAddr, rxTime time.Time) {
	seqID := binary.BigEndian.Uint16(data[30:32])
	var sourceID [10]byte
	copy(sourceID[:], data[20:30])

	// Send PDelay_Resp
	gm.sendPDelayResp(addr, seqID, sourceID, rxTime)
}

// sendAnnounce sends an Announce message.
func (gm *Grandmaster) sendAnnounce() {
	gm.mu.Lock()
	gm.sequenceID++
	seqID := gm.sequenceID
	gm.announceCount++
	gm.lastAnnounce = time.Now()
	gm.mu.Unlock()

	// Build Announce message (64 bytes minimum)
	msg := make([]byte, 64)

	// Header
	msg[0] = MsgAnnounce | 0x00 // Transport specific + message type
	msg[1] = 0x02               // Version PTPv2
	binary.BigEndian.PutUint16(msg[2:4], 64) // Message length
	msg[4] = byte(gm.cfg.Domain)
	msg[5] = 0 // Reserved
	binary.BigEndian.PutUint16(msg[6:8], 0) // Flags
	// Correction field (8 bytes) = 0
	// Reserved (4 bytes) = 0
	copy(msg[20:28], gm.clockID[:]) // Source port identity
	binary.BigEndian.PutUint16(msg[28:30], 1) // Source port number
	binary.BigEndian.PutUint16(msg[30:32], seqID)
	msg[32] = 0x05 // Control field (Announce)
	msg[33] = byte(gm.cfg.LogAnnounceInt)

	// Announce payload
	// Origin timestamp (10 bytes)
	now := time.Now()
	secs := now.Unix()
	nsec := now.Nanosecond()
	binary.BigEndian.PutUint16(msg[34:36], uint16(secs>>32))
	binary.BigEndian.PutUint32(msg[36:40], uint32(secs))
	binary.BigEndian.PutUint32(msg[40:44], uint32(nsec))

	// Current UTC offset (2 bytes)
	binary.BigEndian.PutUint16(msg[44:46], 37) // Current TAI-UTC offset

	// Reserved (1 byte)
	msg[46] = 0

	// Grandmaster priority1, clockClass, clockAccuracy, variance
	msg[47] = gm.priority1
	msg[48] = gm.clockClass
	msg[49] = gm.clockAccuracy
	binary.BigEndian.PutUint16(msg[50:52], gm.variance)
	msg[52] = gm.priority2

	// Grandmaster identity
	copy(msg[53:61], gm.clockID[:])

	// Steps removed, time source
	binary.BigEndian.PutUint16(msg[61:63], 0) // Steps removed
	msg[63] = 0x20 // Time source: GPS

	// Send to multicast
	addr := &net.UDPAddr{
		IP:   net.ParseIP(DefaultMulticast),
		Port: GeneralPort,
	}
	gm.generalConn.WriteToUDP(msg, addr)
	gm.metrics.PTPAnnounce.Inc()
}

// sendSync sends a Sync message.
func (gm *Grandmaster) sendSync() {
	gm.mu.Lock()
	gm.sequenceID++
	seqID := gm.sequenceID
	gm.syncCount++
	gm.lastSync = time.Now()
	gm.mu.Unlock()

	txTime := time.Now()

	// Build Sync message (44 bytes)
	msg := make([]byte, 44)

	// Header
	msg[0] = MsgSync | 0x00
	msg[1] = 0x02 // Version PTPv2
	binary.BigEndian.PutUint16(msg[2:4], 44)
	msg[4] = byte(gm.cfg.Domain)
	if gm.cfg.TwoStepFlag {
		binary.BigEndian.PutUint16(msg[6:8], 0x0200) // Two-step flag
	}
	copy(msg[20:28], gm.clockID[:])
	binary.BigEndian.PutUint16(msg[28:30], 1)
	binary.BigEndian.PutUint16(msg[30:32], seqID)
	msg[32] = 0x00 // Control field (Sync)
	msg[33] = byte(gm.cfg.LogSyncInt)

	// Timestamp (for one-step mode)
	if !gm.cfg.TwoStepFlag {
		secs := txTime.Unix()
		nsec := txTime.Nanosecond()
		binary.BigEndian.PutUint16(msg[34:36], uint16(secs>>32))
		binary.BigEndian.PutUint32(msg[36:40], uint32(secs))
		binary.BigEndian.PutUint32(msg[40:44], uint32(nsec))
	}

	// Send
	addr := &net.UDPAddr{
		IP:   net.ParseIP(DefaultMulticast),
		Port: EventPort,
	}
	gm.eventConn.WriteToUDP(msg, addr)
	gm.metrics.PTPSync.Inc()

	// Send Follow_Up for two-step mode
	if gm.cfg.TwoStepFlag {
		gm.sendFollowUp(seqID, txTime)
	}
}

// sendFollowUp sends a Follow_Up message.
func (gm *Grandmaster) sendFollowUp(seqID uint16, preciseTime time.Time) {
	msg := make([]byte, 44)

	msg[0] = MsgFollowUp | 0x00
	msg[1] = 0x02
	binary.BigEndian.PutUint16(msg[2:4], 44)
	msg[4] = byte(gm.cfg.Domain)
	copy(msg[20:28], gm.clockID[:])
	binary.BigEndian.PutUint16(msg[28:30], 1)
	binary.BigEndian.PutUint16(msg[30:32], seqID)
	msg[32] = 0x02 // Control field (Follow_Up)
	msg[33] = byte(gm.cfg.LogSyncInt)

	// Precise origin timestamp
	secs := preciseTime.Unix()
	nsec := preciseTime.Nanosecond()
	binary.BigEndian.PutUint16(msg[34:36], uint16(secs>>32))
	binary.BigEndian.PutUint32(msg[36:40], uint32(secs))
	binary.BigEndian.PutUint32(msg[40:44], uint32(nsec))

	addr := &net.UDPAddr{
		IP:   net.ParseIP(DefaultMulticast),
		Port: GeneralPort,
	}
	gm.generalConn.WriteToUDP(msg, addr)
}

// sendDelayResp sends a Delay_Resp message.
func (gm *Grandmaster) sendDelayResp(addr *net.UDPAddr, seqID uint16, sourceID [10]byte, rxTime time.Time) {
	msg := make([]byte, 54)

	msg[0] = MsgDelayResp | 0x00
	msg[1] = 0x02
	binary.BigEndian.PutUint16(msg[2:4], 54)
	msg[4] = byte(gm.cfg.Domain)
	copy(msg[20:28], gm.clockID[:])
	binary.BigEndian.PutUint16(msg[28:30], 1)
	binary.BigEndian.PutUint16(msg[30:32], seqID)
	msg[32] = 0x03 // Control field (Delay_Resp)
	msg[33] = byte(gm.cfg.LogMinDelayReqInt)

	// Receive timestamp
	secs := rxTime.Unix()
	nsec := rxTime.Nanosecond()
	binary.BigEndian.PutUint16(msg[34:36], uint16(secs>>32))
	binary.BigEndian.PutUint32(msg[36:40], uint32(secs))
	binary.BigEndian.PutUint32(msg[40:44], uint32(nsec))

	// Requesting port identity
	copy(msg[44:54], sourceID[:])

	gm.generalConn.WriteToUDP(msg, addr)
}

// sendPDelayResp sends a Peer_Delay_Resp message.
func (gm *Grandmaster) sendPDelayResp(addr *net.UDPAddr, seqID uint16, sourceID [10]byte, rxTime time.Time) {
	msg := make([]byte, 54)

	msg[0] = MsgPDelayResp | 0x00
	msg[1] = 0x02
	binary.BigEndian.PutUint16(msg[2:4], 54)
	msg[4] = byte(gm.cfg.Domain)
	copy(msg[20:28], gm.clockID[:])
	binary.BigEndian.PutUint16(msg[28:30], 1)
	binary.BigEndian.PutUint16(msg[30:32], seqID)
	msg[32] = 0x05 // Control
	msg[33] = 0x7F

	secs := rxTime.Unix()
	nsec := rxTime.Nanosecond()
	binary.BigEndian.PutUint16(msg[34:36], uint16(secs>>32))
	binary.BigEndian.PutUint32(msg[36:40], uint32(secs))
	binary.BigEndian.PutUint32(msg[40:44], uint32(nsec))

	copy(msg[44:54], sourceID[:])

	peerAddr := &net.UDPAddr{
		IP:   net.ParseIP(PeerMulticast),
		Port: EventPort,
	}
	gm.eventConn.WriteToUDP(msg, peerAddr)
}

// Stop stops the PTP Grandmaster.
func (gm *Grandmaster) Stop() {
	gm.mu.Lock()
	gm.running = false
	gm.portState = StateDisabled
	gm.mu.Unlock()

	if gm.eventConn != nil {
		gm.eventConn.Close()
	}
	if gm.generalConn != nil {
		gm.generalConn.Close()
	}

	gm.log.Info("PTP Grandmaster stopped")
}

// GetStatus returns the current PTP status.
func (gm *Grandmaster) GetStatus() Status {
	gm.mu.RLock()
	defer gm.mu.RUnlock()

	return Status{
		Running:       gm.running,
		PortState:     gm.portState.String(),
		ClockID:       fmt.Sprintf("%x", gm.clockID),
		Domain:        gm.cfg.Domain,
		Priority1:     int(gm.priority1),
		Priority2:     int(gm.priority2),
		ClockClass:    int(gm.clockClass),
		ClockAccuracy: int(gm.clockAccuracy),
		Profile:       gm.cfg.Profile,
		Interface:     gm.cfg.Interface,
		SyncCount:     gm.syncCount,
		AnnounceCount: gm.announceCount,
		DelayReqCount: gm.delayReqCount,
	}
}
