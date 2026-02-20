// Package metrics provides Prometheus metrics collection for SPTime.
// It exposes metrics for NTP, NTS, PTP, GPSDO status and performance.
package metrics

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	once     sync.Once
	instance *Collector
)

// Collector manages all SPTime metrics.
type Collector struct {
	mu sync.RWMutex

	// NTP metrics
	NTPRequests          prometheus.Counter
	NTPResponses         prometheus.Counter
	NTPErrors            prometheus.Counter
	NTPOffset            prometheus.Gauge
	NTPJitter            prometheus.Gauge
	NTPStratum           prometheus.Gauge
	NTPPeerCount         prometheus.Gauge
	NTPPollInterval      prometheus.Gauge
	NTPLastSync          prometheus.Gauge

	// NTS metrics
	NTSKERequests        prometheus.Counter
	NTSKESuccess         prometheus.Counter
	NTSKEFailures        prometheus.Counter
	NTSAuthSuccess       prometheus.Counter
	NTSAuthFailures      prometheus.Counter
	NTSActiveSessions    prometheus.Gauge

	// PTP metrics
	PTPAnnounce          prometheus.Counter
	PTPSync              prometheus.Counter
	PTPDelayReq          prometheus.Counter
	PTPOffset            prometheus.Gauge
	PTPPathDelay         prometheus.Gauge
	PTPState             prometheus.GaugeVec
	PTPClockClass        prometheus.Gauge
	PTPPortState         prometheus.Gauge

	// GPSDO metrics
	GPSDOLocked          prometheus.Gauge
	GPSDOSatellites      prometheus.Gauge
	GPSDOOffset          prometheus.Gauge
	GPSDOSignalStrength  prometheus.Gauge
	GPSDOPPSCount        prometheus.Counter

	// Clock metrics
	ClockState           prometheus.GaugeVec
	ClockOffset          prometheus.Gauge
	ClockDrift           prometheus.Gauge
	ClockLastUpdate      prometheus.Gauge

	// System metrics
	UptimeSeconds        prometheus.Gauge
	StartTime            time.Time

	// Shadow values for GetSnapshot (updated when Set* methods are called)
	shadowNTPOffset      float64
	shadowNTPJitter      float64
	shadowNTPStratum     int
	shadowNTSSessions    float64
	shadowPTPOffset      float64
	shadowPTPPathDelay   float64
	shadowGPSDOLocked    bool
	shadowGPSDOSats      int
	shadowClockState     string
}

// New creates a new metrics collector and registers all metrics.
// Uses singleton pattern to avoid duplicate registration in tests.
func New() *Collector {
	once.Do(func() {
		instance = createCollector()
	})
	return instance
}

// createCollector creates the actual collector (called once).
func createCollector() *Collector {
	c := &Collector{
		StartTime: time.Now(),

		// NTP metrics
		NTPRequests: promauto.NewCounter(prometheus.CounterOpts{
			Name: "sptime_ntp_requests_total",
			Help: "Total number of NTP requests received",
		}),
		NTPResponses: promauto.NewCounter(prometheus.CounterOpts{
			Name: "sptime_ntp_responses_total",
			Help: "Total number of NTP responses sent",
		}),
		NTPErrors: promauto.NewCounter(prometheus.CounterOpts{
			Name: "sptime_ntp_errors_total",
			Help: "Total number of NTP errors",
		}),
		NTPOffset: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "sptime_ntp_offset_seconds",
			Help: "Current NTP offset in seconds",
		}),
		NTPJitter: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "sptime_ntp_jitter_seconds",
			Help: "Current NTP jitter in seconds",
		}),
		NTPStratum: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "sptime_ntp_stratum",
			Help: "Current NTP stratum level",
		}),
		NTPPeerCount: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "sptime_ntp_peer_count",
			Help: "Number of configured NTP peers",
		}),
		NTPPollInterval: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "sptime_ntp_poll_interval_seconds",
			Help: "Current NTP poll interval in seconds",
		}),
		NTPLastSync: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "sptime_ntp_last_sync_timestamp",
			Help: "Unix timestamp of last successful sync",
		}),

		// NTS metrics
		NTSKERequests: promauto.NewCounter(prometheus.CounterOpts{
			Name: "sptime_nts_ke_requests_total",
			Help: "Total NTS-KE requests",
		}),
		NTSKESuccess: promauto.NewCounter(prometheus.CounterOpts{
			Name: "sptime_nts_ke_success_total",
			Help: "Successful NTS-KE exchanges",
		}),
		NTSKEFailures: promauto.NewCounter(prometheus.CounterOpts{
			Name: "sptime_nts_ke_failures_total",
			Help: "Failed NTS-KE exchanges",
		}),
		NTSAuthSuccess: promauto.NewCounter(prometheus.CounterOpts{
			Name: "sptime_nts_auth_success_total",
			Help: "Successful NTS authenticated requests",
		}),
		NTSAuthFailures: promauto.NewCounter(prometheus.CounterOpts{
			Name: "sptime_nts_auth_failures_total",
			Help: "Failed NTS authentication attempts",
		}),
		NTSActiveSessions: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "sptime_nts_active_sessions",
			Help: "Number of active NTS sessions",
		}),

		// PTP metrics
		PTPAnnounce: promauto.NewCounter(prometheus.CounterOpts{
			Name: "sptime_ptp_announce_total",
			Help: "Total PTP Announce messages sent",
		}),
		PTPSync: promauto.NewCounter(prometheus.CounterOpts{
			Name: "sptime_ptp_sync_total",
			Help: "Total PTP Sync messages sent",
		}),
		PTPDelayReq: promauto.NewCounter(prometheus.CounterOpts{
			Name: "sptime_ptp_delay_req_total",
			Help: "Total PTP Delay_Req messages received",
		}),
		PTPOffset: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "sptime_ptp_offset_nanoseconds",
			Help: "Current PTP offset in nanoseconds",
		}),
		PTPPathDelay: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "sptime_ptp_path_delay_nanoseconds",
			Help: "Current PTP path delay in nanoseconds",
		}),
		PTPState: *promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "sptime_ptp_state",
			Help: "PTP port state (1=active, 0=inactive)",
		}, []string{"state"}),
		PTPClockClass: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "sptime_ptp_clock_class",
			Help: "Current PTP clock class",
		}),
		PTPPortState: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "sptime_ptp_port_state",
			Help: "PTP port state numeric value",
		}),

		// GPSDO metrics
		GPSDOLocked: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "sptime_gpsdo_locked",
			Help: "GPSDO lock status (1=locked, 0=unlocked)",
		}),
		GPSDOSatellites: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "sptime_gpsdo_satellites",
			Help: "Number of visible GPS satellites",
		}),
		GPSDOOffset: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "sptime_gpsdo_offset_nanoseconds",
			Help: "GPSDO offset from GPS time in nanoseconds",
		}),
		GPSDOSignalStrength: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "sptime_gpsdo_signal_strength",
			Help: "Average GPS signal strength (dB)",
		}),
		GPSDOPPSCount: promauto.NewCounter(prometheus.CounterOpts{
			Name: "sptime_gpsdo_pps_total",
			Help: "Total PPS pulses received",
		}),

		// Clock metrics
		ClockState: *promauto.NewGaugeVec(prometheus.GaugeOpts{
			Name: "sptime_clock_state",
			Help: "Clock state (1=active for that state)",
		}, []string{"state"}),
		ClockOffset: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "sptime_clock_offset_nanoseconds",
			Help: "Current clock offset from reference",
		}),
		ClockDrift: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "sptime_clock_drift_ppb",
			Help: "Current clock drift in parts per billion",
		}),
		ClockLastUpdate: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "sptime_clock_last_update_timestamp",
			Help: "Unix timestamp of last clock update",
		}),

		// System metrics
		UptimeSeconds: promauto.NewGauge(prometheus.GaugeOpts{
			Name: "sptime_uptime_seconds",
			Help: "Server uptime in seconds",
		}),
	}

	// Start uptime updater
	go c.updateUptime()

	return c
}

// updateUptime periodically updates the uptime metric.
func (c *Collector) updateUptime() {
	ticker := time.NewTicker(1 * time.Second)
	for range ticker.C {
		c.UptimeSeconds.Set(time.Since(c.StartTime).Seconds())
	}
}

// GetSnapshot returns a snapshot of key metrics for the API.
type MetricsSnapshot struct {
	Uptime          float64 `json:"uptime_seconds"`
	NTPRequests     float64 `json:"ntp_requests"`
	NTPOffset       float64 `json:"ntp_offset_ns"`
	NTPJitter       float64 `json:"ntp_jitter_ns"`
	NTPStratum      int     `json:"ntp_stratum"`
	NTSActiveSess   float64 `json:"nts_active_sessions"`
	PTPOffset       float64 `json:"ptp_offset_ns"`
	PTPPathDelay    float64 `json:"ptp_path_delay_ns"`
	GPSDOLocked     bool    `json:"gpsdo_locked"`
	GPSDOSatellites int     `json:"gpsdo_satellites"`
	ClockState      string  `json:"clock_state"`
}

// GetSnapshot returns current metric values.
func (c *Collector) GetSnapshot() MetricsSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return MetricsSnapshot{
		Uptime:          time.Since(c.StartTime).Seconds(),
		NTPOffset:       c.shadowNTPOffset,
		NTPJitter:       c.shadowNTPJitter,
		NTPStratum:      c.shadowNTPStratum,
		NTSActiveSess:   c.shadowNTSSessions,
		PTPOffset:       c.shadowPTPOffset,
		PTPPathDelay:    c.shadowPTPPathDelay,
		GPSDOLocked:     c.shadowGPSDOLocked,
		GPSDOSatellites: c.shadowGPSDOSats,
		ClockState:      c.shadowClockState,
	}
}

// RecordNTPRequest increments the NTP request counter.
func (c *Collector) RecordNTPRequest() {
	c.NTPRequests.Inc()
}

// RecordNTPResponse increments the NTP response counter.
func (c *Collector) RecordNTPResponse() {
	c.NTPResponses.Inc()
}

// RecordNTPError increments the NTP error counter.
func (c *Collector) RecordNTPError() {
	c.NTPErrors.Inc()
}

// SetNTPOffset sets the current NTP offset.
func (c *Collector) SetNTPOffset(offsetNs float64) {
	c.NTPOffset.Set(offsetNs / 1e9) // Convert ns to seconds
	c.mu.Lock()
	c.shadowNTPOffset = offsetNs
	c.mu.Unlock()
}

// SetNTPJitter sets the current NTP jitter.
func (c *Collector) SetNTPJitter(jitterNs float64) {
	c.NTPJitter.Set(jitterNs / 1e9)
	c.mu.Lock()
	c.shadowNTPJitter = jitterNs
	c.mu.Unlock()
}

// SetNTPStratum sets the current stratum level.
func (c *Collector) SetNTPStratum(stratum int) {
	c.NTPStratum.Set(float64(stratum))
	c.mu.Lock()
	c.shadowNTPStratum = stratum
	c.mu.Unlock()
}

// SetNTSActiveSessions sets the active NTS session count.
func (c *Collector) SetNTSActiveSessions(count float64) {
	c.NTSActiveSessions.Set(count)
	c.mu.Lock()
	c.shadowNTSSessions = count
	c.mu.Unlock()
}

// SetPTPOffset sets the current PTP offset.
func (c *Collector) SetPTPOffset(offsetNs float64) {
	c.PTPOffset.Set(offsetNs)
	c.mu.Lock()
	c.shadowPTPOffset = offsetNs
	c.mu.Unlock()
}

// SetPTPPathDelay sets the current PTP path delay.
func (c *Collector) SetPTPPathDelay(delayNs float64) {
	c.PTPPathDelay.Set(delayNs)
	c.mu.Lock()
	c.shadowPTPPathDelay = delayNs
	c.mu.Unlock()
}

// SetGPSDOLocked sets the GPSDO lock status.
func (c *Collector) SetGPSDOLocked(locked bool) {
	if locked {
		c.GPSDOLocked.Set(1)
	} else {
		c.GPSDOLocked.Set(0)
	}
	c.mu.Lock()
	c.shadowGPSDOLocked = locked
	c.mu.Unlock()
}

// SetGPSDOSatellites sets the visible satellite count.
func (c *Collector) SetGPSDOSatellites(count int) {
	c.GPSDOSatellites.Set(float64(count))
	c.mu.Lock()
	c.shadowGPSDOSats = count
	c.mu.Unlock()
}

// SetClockState sets the current clock state.
func (c *Collector) SetClockState(state string) {
	states := []string{"init", "syncing", "locked", "holdover", "fault"}
	for _, s := range states {
		if s == state {
			c.ClockState.WithLabelValues(s).Set(1)
		} else {
			c.ClockState.WithLabelValues(s).Set(0)
		}
	}
	c.mu.Lock()
	c.shadowClockState = state
	c.mu.Unlock()
}

// RecordPPSPulse increments the PPS counter.
func (c *Collector) RecordPPSPulse() {
	c.GPSDOPPSCount.Inc()
}
