// Package ntp provides NTP server functionality for SPTime.
// Implements RFC 5905 (NTPv4) with support for upstream peers and
// local clock discipline integration.
package ntp

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
	// NTP timestamp epoch: January 1, 1900
	ntpEpoch = 2208988800

	// NTP packet size
	ntpPacketSize = 48

	// NTP versions
	NTPv3 = 3
	NTPv4 = 4

	// NTP modes
	ModeReserved         = 0
	ModeSymmetricActive  = 1
	ModeSymmetricPassive = 2
	ModeClient           = 3
	ModeServer           = 4
	ModeBroadcast        = 5
	ModeControl          = 6
	ModePrivate          = 7

	// Leap indicator
	LeapNoWarning = 0
	LeapInsert    = 1
	LeapDelete    = 2
	LeapUnknown   = 3
)

// Packet represents an NTP packet (48 bytes minimum).
type Packet struct {
	LiVnMode       uint8     // Leap Indicator (2) + Version (3) + Mode (3)
	Stratum        uint8     // Stratum level
	Poll           int8      // Poll interval (log2 seconds)
	Precision      int8      // Precision (log2 seconds)
	RootDelay      uint32    // Root delay (NTP short format)
	RootDispersion uint32    // Root dispersion (NTP short format)
	ReferenceID    [4]byte   // Reference identifier
	RefTimestamp   uint64    // Reference timestamp
	OrigTimestamp  uint64    // Origin timestamp (client's transmit time)
	RxTimestamp    uint64    // Receive timestamp
	TxTimestamp    uint64    // Transmit timestamp
}

// Server implements an NTP server.
type Server struct {
	mu sync.RWMutex

	cfg      config.NTPConfig
	clock    *clock.Manager
	log      *logging.Logger
	metrics  *metrics.Collector

	conn     *net.UDPConn
	peers    []*Peer
	running  bool

	// Server state
	stratum    uint8
	refID      [4]byte
	precision  int8
	rootDelay  time.Duration
	rootDisp   time.Duration
	lastSync   time.Time
}

// Status represents NTP server status.
type Status struct {
	Running        bool          `json:"running"`
	Stratum        int           `json:"stratum"`
	RefID          string        `json:"ref_id"`
	RootDelay      float64       `json:"root_delay_ms"`
	RootDispersion float64       `json:"root_dispersion_ms"`
	Precision      int           `json:"precision"`
	PeerCount      int           `json:"peer_count"`
	LastSync       time.Time     `json:"last_sync"`
	Offset         float64       `json:"offset_us"`
	Jitter         float64       `json:"jitter_us"`
	RequestCount   uint64        `json:"request_count"`
}

// NewServer creates a new NTP server.
func NewServer(cfg config.NTPConfig, clockMgr *clock.Manager, log *logging.Logger, m *metrics.Collector) (*Server, error) {
	s := &Server{
		cfg:       cfg,
		clock:     clockMgr,
		log:       log.WithField("module", "ntp"),
		metrics:   m,
		stratum:   uint8(cfg.Stratum),
		precision: -20, // ~1 microsecond precision
	}

	// Set reference ID
	copy(s.refID[:], cfg.RefID)

	// Initialise peers
	for _, peerCfg := range cfg.UpstreamPeers {
		peer, err := NewPeer(peerCfg, s, log)
		if err != nil {
			log.Warn("Failed to create peer", "address", peerCfg.Address, "error", err)
			continue
		}
		s.peers = append(s.peers, peer)
	}

	m.NTPPeerCount.Set(float64(len(s.peers)))
	m.NTPStratum.Set(float64(s.stratum))

	return s, nil
}

// Start starts the NTP server.
func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf("%s:%d", s.cfg.Interface, s.cfg.Port)
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return fmt.Errorf("failed to resolve address: %w", err)
	}

	s.conn, err = net.ListenUDP("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on UDP: %w", err)
	}

	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	// Start peer polling
	for _, peer := range s.peers {
		go peer.Run(ctx)
	}

	// Main server loop
	go s.serve(ctx)

	// Wait for context cancellation
	<-ctx.Done()
	return nil
}

// serve handles incoming NTP requests.
func (s *Server) serve(ctx context.Context) {
	buf := make([]byte, 512)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Set read deadline to allow periodic context checks
		s.conn.SetReadDeadline(time.Now().Add(1 * time.Second))

		n, addr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			s.log.Error("Read error", "error", err)
			continue
		}

		if n < ntpPacketSize {
			s.log.Debug("Packet too small", "size", n)
			continue
		}

		s.metrics.RecordNTPRequest()

		// Handle request in goroutine
		go s.handleRequest(buf[:n], addr)
	}
}

// handleRequest processes an NTP request and sends a response.
func (s *Server) handleRequest(data []byte, addr *net.UDPAddr) {
	rxTime := time.Now()

	// Parse incoming packet
	req, err := parsePacket(data)
	if err != nil {
		s.log.Debug("Failed to parse packet", "error", err)
		s.metrics.RecordNTPError()
		return
	}

	// Validate request
	mode := req.LiVnMode & 0x07
	if mode != ModeClient {
		s.log.Debug("Non-client mode request", "mode", mode)
		return
	}

	// Build response
	resp := s.buildResponse(req, rxTime)

	// Serialize and send
	respData := serializePacket(resp)
	if _, err := s.conn.WriteToUDP(respData, addr); err != nil {
		s.log.Error("Failed to send response", "error", err)
		s.metrics.RecordNTPError()
		return
	}

	s.metrics.RecordNTPResponse()
}

// buildResponse creates an NTP response packet.
func (s *Server) buildResponse(req *Packet, rxTime time.Time) *Packet {
	s.mu.RLock()
	defer s.mu.RUnlock()

	txTime := time.Now()

	// Determine leap indicator based on clock state
	li := LeapNoWarning
	if s.clock.GetState() != clock.StateLocked {
		li = LeapUnknown
	}

	resp := &Packet{
		LiVnMode:       uint8(li<<6) | uint8(NTPv4<<3) | ModeServer,
		Stratum:        s.stratum,
		Poll:           req.Poll,
		Precision:      s.precision,
		RootDelay:      durationToNTPShort(s.rootDelay),
		RootDispersion: durationToNTPShort(s.rootDisp),
		ReferenceID:    s.refID,
		RefTimestamp:   timeToNTP(s.lastSync),
		OrigTimestamp:  req.TxTimestamp,
		RxTimestamp:    timeToNTP(rxTime),
		TxTimestamp:    timeToNTP(txTime),
	}

	return resp
}

// parsePacket parses raw bytes into an NTP packet.
func parsePacket(data []byte) (*Packet, error) {
	if len(data) < ntpPacketSize {
		return nil, fmt.Errorf("packet too small: %d bytes", len(data))
	}

	p := &Packet{
		LiVnMode:       data[0],
		Stratum:        data[1],
		Poll:           int8(data[2]),
		Precision:      int8(data[3]),
		RootDelay:      binary.BigEndian.Uint32(data[4:8]),
		RootDispersion: binary.BigEndian.Uint32(data[8:12]),
	}
	copy(p.ReferenceID[:], data[12:16])
	p.RefTimestamp = binary.BigEndian.Uint64(data[16:24])
	p.OrigTimestamp = binary.BigEndian.Uint64(data[24:32])
	p.RxTimestamp = binary.BigEndian.Uint64(data[32:40])
	p.TxTimestamp = binary.BigEndian.Uint64(data[40:48])

	return p, nil
}

// serializePacket converts an NTP packet to bytes.
func serializePacket(p *Packet) []byte {
	data := make([]byte, ntpPacketSize)
	data[0] = p.LiVnMode
	data[1] = p.Stratum
	data[2] = byte(p.Poll)
	data[3] = byte(p.Precision)
	binary.BigEndian.PutUint32(data[4:8], p.RootDelay)
	binary.BigEndian.PutUint32(data[8:12], p.RootDispersion)
	copy(data[12:16], p.ReferenceID[:])
	binary.BigEndian.PutUint64(data[16:24], p.RefTimestamp)
	binary.BigEndian.PutUint64(data[24:32], p.OrigTimestamp)
	binary.BigEndian.PutUint64(data[32:40], p.RxTimestamp)
	binary.BigEndian.PutUint64(data[40:48], p.TxTimestamp)
	return data
}

// timeToNTP converts a Go time to NTP timestamp format.
func timeToNTP(t time.Time) uint64 {
	secs := uint64(t.Unix() + ntpEpoch)
	frac := uint64(t.Nanosecond()) * (1 << 32) / 1e9
	return (secs << 32) | frac
}

// ntpToTime converts an NTP timestamp to Go time.
func ntpToTime(ntp uint64) time.Time {
	secs := int64(ntp>>32) - ntpEpoch
	frac := ntp & 0xFFFFFFFF
	nsec := int64(frac) * 1e9 / (1 << 32)
	return time.Unix(secs, nsec)
}

// durationToNTPShort converts a duration to NTP short format (16.16 fixed point).
func durationToNTPShort(d time.Duration) uint32 {
	secs := d.Seconds()
	return uint32(secs * 65536)
}

// ntpShortToDuration converts NTP short format to duration.
func ntpShortToDuration(s uint32) time.Duration {
	secs := float64(s) / 65536
	return time.Duration(secs * float64(time.Second))
}

// Stop stops the NTP server.
func (s *Server) Stop() {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	if s.conn != nil {
		s.conn.Close()
	}

	// Stop peers
	for _, peer := range s.peers {
		peer.Stop()
	}

	s.log.Info("NTP server stopped")
}

// GetStatus returns the current NTP server status.
func (s *Server) GetStatus() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return Status{
		Running:        s.running,
		Stratum:        int(s.stratum),
		RefID:          string(s.refID[:]),
		RootDelay:      s.rootDelay.Seconds() * 1000,
		RootDispersion: s.rootDisp.Seconds() * 1000,
		Precision:      int(s.precision),
		PeerCount:      len(s.peers),
		LastSync:       s.lastSync,
		Offset:         float64(s.clock.GetOffset()) / float64(time.Microsecond),
	}
}

// GetPeers returns the list of peers with their status.
func (s *Server) GetPeers() []PeerStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	statuses := make([]PeerStatus, len(s.peers))
	for i, peer := range s.peers {
		statuses[i] = peer.GetStatus()
	}
	return statuses
}

// UpdateFromPeers updates server state from peer measurements.
func (s *Server) UpdateFromPeers() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find the best peer
	var bestPeer *Peer
	var bestStratum uint8 = 16

	for _, peer := range s.peers {
		status := peer.GetStatus()
		if status.Reachable && status.Stratum < int(bestStratum) {
			bestPeer = peer
			bestStratum = uint8(status.Stratum)
		}
	}

	if bestPeer != nil {
		status := bestPeer.GetStatus()
		s.stratum = bestStratum + 1
		s.rootDelay = time.Duration(status.Delay * float64(time.Microsecond))
		s.rootDisp = time.Duration(status.Jitter * float64(time.Microsecond))
		s.lastSync = time.Now()

		// Update metrics
		s.metrics.SetNTPStratum(int(s.stratum))
		s.metrics.SetNTPOffset(status.Offset * 1000) // us to ns
		s.metrics.SetNTPJitter(status.Jitter * 1000)
		s.metrics.NTPLastSync.Set(float64(s.lastSync.Unix()))
	}
}

// SetStratum sets the server stratum (used when GPSDO is primary).
func (s *Server) SetStratum(stratum int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stratum = uint8(stratum)
	s.metrics.SetNTPStratum(stratum)
}

// SetRefID sets the reference ID.
func (s *Server) SetRefID(refID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	copy(s.refID[:], refID)
}
