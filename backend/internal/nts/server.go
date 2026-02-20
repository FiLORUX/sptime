// Package nts implements Network Time Security (NTS) as defined in RFC 8915.
// It provides NTS-KE (Key Establishment) server and NTS-authenticated NTP support.
package nts

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/sptime/sptime/internal/clock"
	"github.com/sptime/sptime/internal/config"
	"github.com/sptime/sptime/internal/logging"
	"github.com/sptime/sptime/internal/metrics"
)

const (
	// NTS-KE record types
	RecordEndOfMessage      = 0
	RecordNextProtocol      = 1
	RecordError             = 2
	RecordWarning           = 3
	RecordAEADAlgorithm     = 4
	RecordCookie            = 5
	RecordServer            = 6
	RecordPort              = 7

	// Next protocol IDs
	ProtocolNTPv4 = 0

	// AEAD algorithm IDs
	AEADAlgorithmAESGCM128 = 15

	// NTS extension field types
	ExtUniqueID        = 0x0104
	ExtNTSCookie       = 0x0204
	ExtNTSCookiePlaceholder = 0x0304
	ExtNTSAuth         = 0x0404

	// Cookie size
	CookieSize = 100
	CookieKeySize = 32
	NonceSize = 16
)

// Server implements an NTS-KE server.
type Server struct {
	mu sync.RWMutex

	cfg       config.NTSConfig
	clock     *clock.Manager
	log       *logging.Logger
	metrics   *metrics.Collector

	listener  net.Listener
	tlsConfig *tls.Config
	running   bool

	// Cookie encryption
	cookieKey []byte
	aead      cipher.AEAD

	// Session tracking
	sessions  map[string]*Session
}

// Session represents an active NTS session.
type Session struct {
	ID         string
	C2S        []byte // Client-to-server key
	S2C        []byte // Server-to-client key
	Created    time.Time
	LastUsed   time.Time
	UseCount   int
}

// Status represents NTS server status.
type Status struct {
	Running        bool   `json:"running"`
	KEPort         int    `json:"ke_port"`
	ActiveSessions int    `json:"active_sessions"`
	TotalKEReqs    uint64 `json:"total_ke_requests"`
	TotalSuccess   uint64 `json:"total_ke_success"`
	TotalFailures  uint64 `json:"total_ke_failures"`
	CertExpiry     string `json:"cert_expiry"`
}

// NewServer creates a new NTS server.
func NewServer(cfg config.NTSConfig, clockMgr *clock.Manager, log *logging.Logger, m *metrics.Collector) (*Server, error) {
	// Load TLS certificate
	cert, err := tls.LoadX509KeyPair(cfg.CertPath, cfg.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS certificate: %w", err)
	}

	// Configure TLS
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
		NextProtos:   []string{"ntske/1"},
	}

	// Generate or load cookie key
	cookieKey := make([]byte, CookieKeySize)
	if cfg.CookieKey != "" {
		// Use configured key (should be hex-encoded in config)
		copy(cookieKey, []byte(cfg.CookieKey))
	} else {
		// Generate random key
		if _, err := rand.Read(cookieKey); err != nil {
			return nil, fmt.Errorf("failed to generate cookie key: %w", err)
		}
	}

	// Create AES-GCM AEAD
	block, err := aes.NewCipher(cookieKey[:16])
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	return &Server{
		cfg:       cfg,
		clock:     clockMgr,
		log:       log.WithField("module", "nts"),
		metrics:   m,
		tlsConfig: tlsConfig,
		cookieKey: cookieKey,
		aead:      aead,
		sessions:  make(map[string]*Session),
	}, nil
}

// Start starts the NTS-KE server.
func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", s.cfg.KEPort)

	var err error
	s.listener, err = tls.Listen("tcp", addr, s.tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to start NTS-KE listener: %w", err)
	}

	s.mu.Lock()
	s.running = true
	s.mu.Unlock()

	s.log.Info("NTS-KE server started", "port", s.cfg.KEPort)

	// Start session cleanup goroutine
	go s.cleanupSessions(ctx)

	// Accept connections
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		conn, err := s.listener.Accept()
		if err != nil {
			if !s.running {
				return nil
			}
			s.log.Error("Accept error", "error", err)
			continue
		}

		go s.handleConnection(conn.(*tls.Conn))
	}
}

// handleConnection handles a single NTS-KE connection.
func (s *Server) handleConnection(conn *tls.Conn) {
	defer conn.Close()

	s.metrics.NTSKERequests.Inc()

	// Set deadline
	conn.SetDeadline(time.Now().Add(30 * time.Second))

	// Perform TLS handshake
	if err := conn.Handshake(); err != nil {
		s.log.Debug("TLS handshake failed", "error", err)
		s.metrics.NTSKEFailures.Inc()
		return
	}

	// Read NTS-KE request
	req, err := s.readRequest(conn)
	if err != nil {
		s.log.Debug("Failed to read NTS-KE request", "error", err)
		s.metrics.NTSKEFailures.Inc()
		return
	}

	// Process request
	resp, err := s.processRequest(conn, req)
	if err != nil {
		s.log.Debug("Failed to process NTS-KE request", "error", err)
		s.metrics.NTSKEFailures.Inc()
		return
	}

	// Send response
	if err := s.writeResponse(conn, resp); err != nil {
		s.log.Debug("Failed to write NTS-KE response", "error", err)
		s.metrics.NTSKEFailures.Inc()
		return
	}

	s.metrics.NTSKESuccess.Inc()
	s.log.Debug("NTS-KE exchange successful", "remote", conn.RemoteAddr())
}

// Record represents an NTS-KE record.
type Record struct {
	Critical bool
	Type     uint16
	Body     []byte
}

// Request represents an NTS-KE request.
type Request struct {
	Records []Record
}

// Response represents an NTS-KE response.
type Response struct {
	Records []Record
}

// readRequest reads NTS-KE records from the connection.
func (s *Server) readRequest(conn net.Conn) (*Request, error) {
	req := &Request{}

	for {
		// Read record header (4 bytes)
		header := make([]byte, 4)
		if _, err := io.ReadFull(conn, header); err != nil {
			return nil, fmt.Errorf("failed to read record header: %w", err)
		}

		critical := (header[0] & 0x80) != 0
		recType := binary.BigEndian.Uint16(header[0:2]) & 0x7FFF
		bodyLen := binary.BigEndian.Uint16(header[2:4])

		// Read record body
		body := make([]byte, bodyLen)
		if bodyLen > 0 {
			if _, err := io.ReadFull(conn, body); err != nil {
				return nil, fmt.Errorf("failed to read record body: %w", err)
			}
		}

		record := Record{
			Critical: critical,
			Type:     recType,
			Body:     body,
		}
		req.Records = append(req.Records, record)

		// End of message
		if recType == RecordEndOfMessage {
			break
		}
	}

	return req, nil
}

// processRequest processes an NTS-KE request and generates a response.
func (s *Server) processRequest(conn *tls.Conn, req *Request) (*Response, error) {
	resp := &Response{}

	// Validate request has required records
	hasNextProtocol := false
	hasAEAD := false
	requestedAEAD := uint16(AEADAlgorithmAESGCM128)

	for _, rec := range req.Records {
		switch rec.Type {
		case RecordNextProtocol:
			if len(rec.Body) >= 2 {
				proto := binary.BigEndian.Uint16(rec.Body)
				if proto == ProtocolNTPv4 {
					hasNextProtocol = true
				}
			}
		case RecordAEADAlgorithm:
			if len(rec.Body) >= 2 {
				requestedAEAD = binary.BigEndian.Uint16(rec.Body)
				if requestedAEAD == AEADAlgorithmAESGCM128 {
					hasAEAD = true
				}
			}
		}
	}

	if !hasNextProtocol {
		// Send error
		errorBody := make([]byte, 2)
		binary.BigEndian.PutUint16(errorBody, 1) // Bad request
		resp.Records = append(resp.Records, Record{
			Critical: true,
			Type:     RecordError,
			Body:     errorBody,
		})
		resp.Records = append(resp.Records, Record{
			Critical: true,
			Type:     RecordEndOfMessage,
		})
		return resp, nil
	}

	// Add Next Protocol response
	protoBody := make([]byte, 2)
	binary.BigEndian.PutUint16(protoBody, ProtocolNTPv4)
	resp.Records = append(resp.Records, Record{
		Critical: true,
		Type:     RecordNextProtocol,
		Body:     protoBody,
	})

	// Add AEAD algorithm
	if hasAEAD {
		aeadBody := make([]byte, 2)
		binary.BigEndian.PutUint16(aeadBody, requestedAEAD)
		resp.Records = append(resp.Records, Record{
			Critical: true,
			Type:     RecordAEADAlgorithm,
			Body:     aeadBody,
		})
	}

	// Export keys from TLS session
	state := conn.ConnectionState()
	c2s := make([]byte, 32)
	s2c := make([]byte, 32)

	// Use TLS 1.3 exporter for key derivation
	c2sLabel := "EXPORTER-network-time-security/1"
	s2cLabel := "EXPORTER-network-time-security/1"

	var err error
	c2s, err = state.ExportKeyingMaterial(c2sLabel, []byte{0x00, 0x00, 0x00, 0x0f, 0x00}, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to export C2S key: %w", err)
	}
	s2c, err = state.ExportKeyingMaterial(s2cLabel, []byte{0x00, 0x00, 0x00, 0x0f, 0x01}, 32)
	if err != nil {
		return nil, fmt.Errorf("failed to export S2C key: %w", err)
	}

	// Create session
	session := &Session{
		ID:       generateSessionID(),
		C2S:      c2s,
		S2C:      s2c,
		Created:  time.Now(),
		LastUsed: time.Now(),
	}

	s.mu.Lock()
	s.sessions[session.ID] = session
	s.metrics.NTSActiveSessions.Set(float64(len(s.sessions)))
	s.mu.Unlock()

	// Generate cookies (8 by default)
	for i := 0; i < 8; i++ {
		cookie, err := s.generateCookie(session)
		if err != nil {
			return nil, fmt.Errorf("failed to generate cookie: %w", err)
		}
		resp.Records = append(resp.Records, Record{
			Critical: false,
			Type:     RecordCookie,
			Body:     cookie,
		})
	}

	// End of message
	resp.Records = append(resp.Records, Record{
		Critical: true,
		Type:     RecordEndOfMessage,
	})

	return resp, nil
}

// writeResponse writes NTS-KE records to the connection.
func (s *Server) writeResponse(conn net.Conn, resp *Response) error {
	for _, rec := range resp.Records {
		header := make([]byte, 4)

		typeVal := rec.Type
		if rec.Critical {
			typeVal |= 0x8000
		}
		binary.BigEndian.PutUint16(header[0:2], typeVal)
		binary.BigEndian.PutUint16(header[2:4], uint16(len(rec.Body)))

		if _, err := conn.Write(header); err != nil {
			return err
		}
		if len(rec.Body) > 0 {
			if _, err := conn.Write(rec.Body); err != nil {
				return err
			}
		}
	}
	return nil
}

// generateCookie generates an encrypted NTS cookie.
func (s *Server) generateCookie(session *Session) ([]byte, error) {
	// Cookie plaintext: session ID + C2S + S2C
	plaintext := make([]byte, 0, 32+32+32)
	plaintext = append(plaintext, []byte(session.ID)...)
	plaintext = append(plaintext, session.C2S...)
	plaintext = append(plaintext, session.S2C...)

	// Generate nonce
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}

	// Encrypt
	ciphertext := s.aead.Seal(nil, nonce, plaintext, nil)

	// Cookie = nonce + ciphertext
	cookie := make([]byte, 0, len(nonce)+len(ciphertext))
	cookie = append(cookie, nonce...)
	cookie = append(cookie, ciphertext...)

	return cookie, nil
}

// DecryptCookie decrypts and validates an NTS cookie.
func (s *Server) DecryptCookie(cookie []byte) (*Session, error) {
	if len(cookie) < s.aead.NonceSize()+s.aead.Overhead() {
		return nil, fmt.Errorf("cookie too short")
	}

	nonce := cookie[:s.aead.NonceSize()]
	ciphertext := cookie[s.aead.NonceSize():]

	plaintext, err := s.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("cookie decryption failed: %w", err)
	}

	if len(plaintext) < 96 {
		return nil, fmt.Errorf("invalid cookie content")
	}

	session := &Session{
		ID:       string(plaintext[:32]),
		C2S:      plaintext[32:64],
		S2C:      plaintext[64:96],
		LastUsed: time.Now(),
	}

	return session, nil
}

// generateSessionID generates a random session ID.
func generateSessionID() string {
	b := make([]byte, 32)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}

// cleanupSessions periodically removes expired sessions.
func (s *Server) cleanupSessions(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			now := time.Now()
			for id, session := range s.sessions {
				if now.Sub(session.LastUsed) > 24*time.Hour {
					delete(s.sessions, id)
				}
			}
			s.metrics.NTSActiveSessions.Set(float64(len(s.sessions)))
			s.mu.Unlock()
		}
	}
}

// Stop stops the NTS server.
func (s *Server) Stop() {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	if s.listener != nil {
		s.listener.Close()
	}

	s.log.Info("NTS server stopped")
}

// GetStatus returns the current NTS server status.
func (s *Server) GetStatus() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return Status{
		Running:        s.running,
		KEPort:         s.cfg.KEPort,
		ActiveSessions: len(s.sessions),
	}
}
