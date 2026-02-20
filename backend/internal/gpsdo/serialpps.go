// Package gpsdo provides SerialPPS implementation for GPSDO with NMEA over serial and PPS signal.
package gpsdo

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sptime/sptime/internal/clock"
	"github.com/sptime/sptime/internal/config"
	"github.com/sptime/sptime/internal/logging"
	"github.com/tarm/serial"
)

// SerialPPS implements TimeReference for GPS receivers with serial NMEA and PPS.
type SerialPPS struct {
	mu sync.RWMutex

	cfg config.GPSDOConfig
	log *logging.Logger

	// Serial connection for NMEA
	serialPort *serial.Port

	// PPS device file
	ppsFile *os.File

	// State
	running       bool
	connected     bool
	locked        bool
	lockStart     time.Time

	// GPS data
	latitude      float64
	longitude     float64
	altitude      float64
	satellites    int
	hdop          float64
	gpsTime       time.Time
	satInfo       []Satellite

	// PPS data
	ppsCount      uint64
	lastPPS       time.Time
	ppsOffset     time.Duration

	// Statistics
	lastNMEA      time.Time
	nmeaCount     uint64
}

// NewSerialPPS creates a new SerialPPS GPSDO instance.
func NewSerialPPS(cfg config.GPSDOConfig, log *logging.Logger) (*SerialPPS, error) {
	return &SerialPPS{
		cfg:      cfg,
		log:      log.WithField("gpsdo", "serialpps"),
		satInfo:  make([]Satellite, 0),
	}, nil
}

// Name returns the reference name.
func (s *SerialPPS) Name() string {
	return "GPSDO:SerialPPS"
}

// Start starts the GPSDO monitoring.
func (s *SerialPPS) Start(ctx context.Context) error {
	// Open serial port for NMEA
	serialCfg := &serial.Config{
		Name:        s.cfg.SerialDevice,
		Baud:        s.cfg.SerialBaud,
		ReadTimeout: time.Second,
	}

	var err error
	s.serialPort, err = serial.OpenPort(serialCfg)
	if err != nil {
		return fmt.Errorf("failed to open serial port %s: %w", s.cfg.SerialDevice, err)
	}

	// Open PPS device
	s.ppsFile, err = os.Open(s.cfg.PPSDevice)
	if err != nil {
		s.serialPort.Close()
		return fmt.Errorf("failed to open PPS device %s: %w", s.cfg.PPSDevice, err)
	}

	s.mu.Lock()
	s.running = true
	s.connected = true
	s.mu.Unlock()

	s.log.Info("SerialPPS GPSDO started",
		"serial", s.cfg.SerialDevice,
		"pps", s.cfg.PPSDevice)

	// Start NMEA reader
	go s.readNMEA(ctx)

	// Start PPS reader
	go s.readPPS(ctx)

	<-ctx.Done()
	return nil
}

// readNMEA continuously reads and parses NMEA sentences.
func (s *SerialPPS) readNMEA(ctx context.Context) {
	reader := bufio.NewReader(s.serialPort)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			s.log.Debug("NMEA read error", "error", err)
			continue
		}

		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "$") {
			continue
		}

		s.parseNMEA(line)
	}
}

// parseNMEA parses an NMEA sentence.
func (s *SerialPPS) parseNMEA(sentence string) {
	// Verify checksum
	if !verifyNMEAChecksum(sentence) {
		return
	}

	// Remove checksum
	if idx := strings.Index(sentence, "*"); idx > 0 {
		sentence = sentence[:idx]
	}

	fields := strings.Split(sentence, ",")
	if len(fields) < 2 {
		return
	}

	sentenceType := fields[0]

	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastNMEA = time.Now()
	s.nmeaCount++

	switch {
	case strings.HasSuffix(sentenceType, "RMC"):
		s.parseRMC(fields)
	case strings.HasSuffix(sentenceType, "GGA"):
		s.parseGGA(fields)
	case strings.HasSuffix(sentenceType, "GSV"):
		s.parseGSV(fields)
	case strings.HasSuffix(sentenceType, "GSA"):
		s.parseGSA(fields)
	}
}

// parseRMC parses the Recommended Minimum sentence.
func (s *SerialPPS) parseRMC(fields []string) {
	if len(fields) < 10 {
		return
	}

	// Status: A=active, V=void
	if fields[2] == "A" {
		s.locked = true
		if s.lockStart.IsZero() {
			s.lockStart = time.Now()
		}
	} else {
		s.locked = false
		s.lockStart = time.Time{}
	}

	// Parse time (HHMMSS.sss)
	if len(fields[1]) >= 6 {
		hour, _ := strconv.Atoi(fields[1][0:2])
		min, _ := strconv.Atoi(fields[1][2:4])
		sec, _ := strconv.Atoi(fields[1][4:6])

		// Parse date (DDMMYY)
		if len(fields[9]) >= 6 {
			day, _ := strconv.Atoi(fields[9][0:2])
			month, _ := strconv.Atoi(fields[9][2:4])
			year, _ := strconv.Atoi(fields[9][4:6])
			year += 2000

			s.gpsTime = time.Date(year, time.Month(month), day, hour, min, sec, 0, time.UTC)
		}
	}

	// Parse latitude
	if len(fields[3]) > 0 && len(fields[4]) > 0 {
		s.latitude = parseNMEACoordinate(fields[3], fields[4])
	}

	// Parse longitude
	if len(fields[5]) > 0 && len(fields[6]) > 0 {
		s.longitude = parseNMEACoordinate(fields[5], fields[6])
	}
}

// parseGGA parses the Global Positioning System Fix Data sentence.
func (s *SerialPPS) parseGGA(fields []string) {
	if len(fields) < 15 {
		return
	}

	// Number of satellites
	if n, err := strconv.Atoi(fields[7]); err == nil {
		s.satellites = n
	}

	// HDOP
	if hdop, err := strconv.ParseFloat(fields[8], 64); err == nil {
		s.hdop = hdop
	}

	// Altitude
	if alt, err := strconv.ParseFloat(fields[9], 64); err == nil {
		s.altitude = alt
	}
}

// parseGSV parses the Satellites in View sentence.
func (s *SerialPPS) parseGSV(fields []string) {
	if len(fields) < 8 {
		return
	}

	// Parse satellite info (4 fields per satellite: PRN, elevation, azimuth, SNR)
	for i := 4; i+3 < len(fields); i += 4 {
		prn, _ := strconv.Atoi(fields[i])
		elev, _ := strconv.Atoi(fields[i+1])
		az, _ := strconv.Atoi(fields[i+2])
		snr, _ := strconv.ParseFloat(fields[i+3], 64)

		if prn > 0 {
			sat := Satellite{
				PRN:       prn,
				Elevation: elev,
				Azimuth:   az,
				SNR:       snr,
			}

			// Update or add satellite
			found := false
			for j, existing := range s.satInfo {
				if existing.PRN == prn {
					s.satInfo[j] = sat
					found = true
					break
				}
			}
			if !found && len(s.satInfo) < 32 {
				s.satInfo = append(s.satInfo, sat)
			}
		}
	}
}

// parseGSA parses the DOP and Active Satellites sentence.
func (s *SerialPPS) parseGSA(fields []string) {
	if len(fields) < 18 {
		return
	}

	// Mark satellites as used
	usedPRNs := make(map[int]bool)
	for i := 3; i < 15 && i < len(fields); i++ {
		if prn, err := strconv.Atoi(fields[i]); err == nil && prn > 0 {
			usedPRNs[prn] = true
		}
	}

	for i := range s.satInfo {
		s.satInfo[i].Used = usedPRNs[s.satInfo[i].PRN]
	}
}

// readPPS reads PPS timestamps.
func (s *SerialPPS) readPPS(ctx context.Context) {
	// PPS reading implementation depends on the kernel driver
	// This is a simplified version that uses file timestamps
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.checkPPS()
		}
	}
}

// checkPPS checks for new PPS events.
func (s *SerialPPS) checkPPS() {
	// Read PPS device (implementation depends on kernel interface)
	// For /dev/pps0, we would use ioctl PPS_FETCH
	// This is a simplified implementation

	info, err := s.ppsFile.Stat()
	if err != nil {
		return
	}

	modTime := info.ModTime()

	s.mu.Lock()
	defer s.mu.Unlock()

	if modTime.After(s.lastPPS) {
		now := time.Now()
		s.lastPPS = now
		s.ppsCount++

		// Calculate offset from PPS to system clock
		// PPS should occur at the second boundary
		nsec := now.Nanosecond()
		if nsec > 500000000 {
			s.ppsOffset = time.Duration(nsec-1000000000) * time.Nanosecond
		} else {
			s.ppsOffset = time.Duration(nsec) * time.Nanosecond
		}
	}
}

// parseNMEACoordinate parses an NMEA coordinate value.
func parseNMEACoordinate(value, direction string) float64 {
	if len(value) < 4 {
		return 0
	}

	// Find decimal point
	dotIdx := strings.Index(value, ".")
	if dotIdx < 2 {
		return 0
	}

	// Parse degrees and minutes
	degLen := dotIdx - 2
	deg, _ := strconv.ParseFloat(value[:degLen], 64)
	min, _ := strconv.ParseFloat(value[degLen:], 64)

	coord := deg + min/60.0

	if direction == "S" || direction == "W" {
		coord = -coord
	}

	return coord
}

// verifyNMEAChecksum verifies the checksum of an NMEA sentence.
func verifyNMEAChecksum(sentence string) bool {
	if len(sentence) < 4 {
		return false
	}

	starIdx := strings.LastIndex(sentence, "*")
	if starIdx < 0 || starIdx+3 > len(sentence) {
		return false
	}

	// Calculate checksum (XOR of all characters between $ and *)
	var checksum byte
	for i := 1; i < starIdx; i++ {
		checksum ^= sentence[i]
	}

	// Parse expected checksum
	expected, err := strconv.ParseUint(sentence[starIdx+1:starIdx+3], 16, 8)
	if err != nil {
		return false
	}

	return checksum == byte(expected)
}

// Stop stops the GPSDO monitoring.
func (s *SerialPPS) Stop() {
	s.mu.Lock()
	s.running = false
	s.connected = false
	s.mu.Unlock()

	if s.serialPort != nil {
		s.serialPort.Close()
	}
	if s.ppsFile != nil {
		s.ppsFile.Close()
	}

	s.log.Info("SerialPPS GPSDO stopped")
}

// GetTime returns the current GPS time.
func (s *SerialPPS) GetTime() (time.Time, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.locked {
		return time.Time{}, fmt.Errorf("GPS not locked")
	}

	return s.gpsTime, nil
}

// GetOffset returns the offset between GPS time and system time.
func (s *SerialPPS) GetOffset() (time.Duration, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.locked {
		return 0, fmt.Errorf("GPS not locked")
	}

	return s.ppsOffset, nil
}

// IsLocked returns true if GPS has a valid fix.
func (s *SerialPPS) IsLocked() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.locked && s.satellites >= 4
}

// Priority returns the reference priority (lower is better).
func (s *SerialPPS) Priority() int {
	return 0 // Highest priority for GPS
}

// Quality returns the reference quality information.
func (s *SerialPPS) Quality() clock.ReferenceQuality {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stratum := 1 // Stratum 1 for GPS
	if !s.locked {
		stratum = 16
	}

	return clock.ReferenceQuality{
		Stratum:    stratum,
		Accuracy:   time.Microsecond, // Typical GPS accuracy
		LastUpdate: s.lastNMEA,
	}
}

// GetStatus returns the current GPSDO status.
func (s *SerialPPS) GetStatus() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var lockDuration time.Duration
	if !s.lockStart.IsZero() {
		lockDuration = time.Since(s.lockStart)
	}

	// Calculate average signal strength
	var avgSNR float64
	if len(s.satInfo) > 0 {
		var sum float64
		for _, sat := range s.satInfo {
			sum += sat.SNR
		}
		avgSNR = sum / float64(len(s.satInfo))
	}

	return Status{
		Type:           "serialpps",
		Connected:      s.connected,
		Locked:         s.locked,
		LockDuration:   lockDuration,
		Satellites:     s.satellites,
		SignalStrength: avgSNR,
		Offset:         s.ppsOffset,
		Latitude:       s.latitude,
		Longitude:      s.longitude,
		Altitude:       s.altitude,
		HDOP:           s.hdop,
		PPSCount:       s.ppsCount,
		LastPPS:        s.lastPPS,
		LastNMEA:       s.lastNMEA,
	}
}

// GetSatellites returns information about visible satellites.
func (s *SerialPPS) GetSatellites() []Satellite {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Satellite, len(s.satInfo))
	copy(result, s.satInfo)
	return result
}
