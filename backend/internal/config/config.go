// Package config handles loading and validation of SPTime configuration.
// Configuration is stored in YAML format and supports hot-reloading.
package config

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete SPTime configuration.
type Config struct {
	mu sync.RWMutex

	NTP     NTPConfig     `yaml:"ntp"`
	NTS     NTSConfig     `yaml:"nts"`
	PTP     PTPConfig     `yaml:"ptp"`
	GPSDO   GPSDOConfig   `yaml:"gpsdo"`
	Web     WebConfig     `yaml:"web"`
	Logging LoggingConfig `yaml:"logging"`
}

// NTPConfig holds NTP server configuration.
type NTPConfig struct {
	Enabled       bool          `yaml:"enabled"`
	Port          int           `yaml:"port"`
	Interface     string        `yaml:"interface"`
	Stratum       int           `yaml:"stratum"`
	PollInterval  time.Duration `yaml:"poll_interval"`
	UpstreamPeers []PeerConfig  `yaml:"upstream_peers"`
	AllowedNets   []string      `yaml:"allowed_networks"`
	RefID         string        `yaml:"ref_id"`
}

// PeerConfig represents an upstream NTP peer.
type PeerConfig struct {
	Address   string `yaml:"address"`
	Prefer    bool   `yaml:"prefer"`
	IBurst    bool   `yaml:"iburst"`
	MinPoll   int    `yaml:"minpoll"`
	MaxPoll   int    `yaml:"maxpoll"`
	NTSEnabled bool  `yaml:"nts_enabled"`
}

// NTSConfig holds NTS (Network Time Security) configuration.
type NTSConfig struct {
	Enabled     bool   `yaml:"enabled"`
	KEPort      int    `yaml:"ke_port"`
	CertPath    string `yaml:"cert_path"`
	KeyPath     string `yaml:"key_path"`
	CAPath      string `yaml:"ca_path"`
	CookieKey   string `yaml:"cookie_key"`
	MinTLSVer   string `yaml:"min_tls_version"`
	CipherSuite string `yaml:"cipher_suite"`
}

// PTPConfig holds PTP Grandmaster configuration.
type PTPConfig struct {
	Enabled           bool   `yaml:"enabled"`
	Interface         string `yaml:"interface"`
	Domain            int    `yaml:"domain"`
	Priority1         int    `yaml:"priority1"`
	Priority2         int    `yaml:"priority2"`
	ClockClass        int    `yaml:"clock_class"`
	ClockAccuracy     int    `yaml:"clock_accuracy"`
	LogAnnounceInt    int    `yaml:"log_announce_interval"`
	LogSyncInt        int    `yaml:"log_sync_interval"`
	LogMinDelayReqInt int    `yaml:"log_min_delay_req_interval"`
	TransportMode     string `yaml:"transport_mode"`
	DelayMechanism    string `yaml:"delay_mechanism"`
	Profile           string `yaml:"profile"`
	TwoStepFlag       bool   `yaml:"two_step_flag"`
}

// GPSDOConfig holds GPSDO (GPS Disciplined Oscillator) configuration.
type GPSDOConfig struct {
	Enabled       bool   `yaml:"enabled"`
	Type          string `yaml:"type"`
	SerialDevice  string `yaml:"serial_device"`
	SerialBaud    int    `yaml:"serial_baud"`
	PPSDevice     string `yaml:"pps_device"`
	NetworkAddr   string `yaml:"network_address"`
	NMEASentences []string `yaml:"nmea_sentences"`
	PollInterval  time.Duration `yaml:"poll_interval"`
}

// WebConfig holds Web API and GUI configuration.
type WebConfig struct {
	Port           int    `yaml:"port"`
	BindAddress    string `yaml:"bind_address"`
	TLSEnabled     bool   `yaml:"tls_enabled"`
	TLSCert        string `yaml:"tls_cert"`
	TLSKey         string `yaml:"tls_key"`
	JWTSecret      string `yaml:"jwt_secret"`
	JWTExpiry      time.Duration `yaml:"jwt_expiry"`
	SessionTimeout time.Duration `yaml:"session_timeout"`
	AllowedOrigins []string `yaml:"allowed_origins"`
	ReadOnlyMode   bool   `yaml:"read_only_mode"`
	Users          []UserConfig `yaml:"users"`
	RateLimitRPS   int    `yaml:"rate_limit_rps"`
	StaticDir      string `yaml:"static_dir"`
}

// UserConfig represents a local user account.
type UserConfig struct {
	Username     string `yaml:"username"`
	PasswordHash string `yaml:"password_hash"`
	Role         string `yaml:"role"`
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level      string `yaml:"level"`
	Format     string `yaml:"format"`
	Output     string `yaml:"output"`
	FilePath   string `yaml:"file_path"`
	MaxSize    int    `yaml:"max_size_mb"`
	MaxBackups int    `yaml:"max_backups"`
	MaxAge     int    `yaml:"max_age_days"`
}

// Load reads and parses configuration from the specified path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Expand environment variables in config (only ${VAR} format to avoid
	// conflicts with bcrypt hashes which contain $ characters)
	expandedData := expandEnvBraces(string(data))

	cfg := &Config{}
	if err := yaml.Unmarshal([]byte(expandedData), cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Apply defaults
	cfg.applyDefaults()

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// applyDefaults sets default values for unspecified configuration options.
func (c *Config) applyDefaults() {
	// NTP defaults
	if c.NTP.Port == 0 {
		c.NTP.Port = 123
	}
	if c.NTP.Stratum == 0 {
		c.NTP.Stratum = 2
	}
	if c.NTP.PollInterval == 0 {
		c.NTP.PollInterval = 64 * time.Second
	}
	if c.NTP.RefID == "" {
		c.NTP.RefID = "LOCL"
	}

	// NTS defaults
	if c.NTS.KEPort == 0 {
		c.NTS.KEPort = 4460
	}
	if c.NTS.MinTLSVer == "" {
		c.NTS.MinTLSVer = "1.3"
	}

	// PTP defaults
	if c.PTP.Domain == 0 {
		c.PTP.Domain = 0
	}
	if c.PTP.Priority1 == 0 {
		c.PTP.Priority1 = 128
	}
	if c.PTP.Priority2 == 0 {
		c.PTP.Priority2 = 128
	}
	if c.PTP.ClockClass == 0 {
		c.PTP.ClockClass = 248
	}
	if c.PTP.ClockAccuracy == 0 {
		c.PTP.ClockAccuracy = 0xFE
	}
	if c.PTP.TransportMode == "" {
		c.PTP.TransportMode = "multicast"
	}
	if c.PTP.DelayMechanism == "" {
		c.PTP.DelayMechanism = "E2E"
	}
	if c.PTP.Profile == "" {
		c.PTP.Profile = "default"
	}

	// GPSDO defaults
	if c.GPSDO.Type == "" {
		c.GPSDO.Type = "dummy"
	}
	if c.GPSDO.SerialBaud == 0 {
		c.GPSDO.SerialBaud = 9600
	}
	if c.GPSDO.PollInterval == 0 {
		c.GPSDO.PollInterval = 1 * time.Second
	}
	if len(c.GPSDO.NMEASentences) == 0 {
		c.GPSDO.NMEASentences = []string{"GPRMC", "GPGGA"}
	}

	// Web defaults
	if c.Web.Port == 0 {
		c.Web.Port = 8080
	}
	if c.Web.BindAddress == "" {
		c.Web.BindAddress = "0.0.0.0"
	}
	if c.Web.JWTExpiry == 0 {
		c.Web.JWTExpiry = 24 * time.Hour
	}
	if c.Web.SessionTimeout == 0 {
		c.Web.SessionTimeout = 30 * time.Minute
	}
	if c.Web.RateLimitRPS == 0 {
		c.Web.RateLimitRPS = 100
	}
	if c.Web.StaticDir == "" {
		c.Web.StaticDir = "./static"
	}

	// Logging defaults
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Logging.Format == "" {
		c.Logging.Format = "json"
	}
	if c.Logging.Output == "" {
		c.Logging.Output = "stdout"
	}
	if c.Logging.MaxSize == 0 {
		c.Logging.MaxSize = 100
	}
	if c.Logging.MaxBackups == 0 {
		c.Logging.MaxBackups = 3
	}
	if c.Logging.MaxAge == 0 {
		c.Logging.MaxAge = 28
	}
}

// Validate checks the configuration for errors.
func (c *Config) Validate() error {
	var errs []error

	// Validate NTP config
	if c.NTP.Enabled {
		if c.NTP.Port < 1 || c.NTP.Port > 65535 {
			errs = append(errs, errors.New("ntp.port must be between 1 and 65535"))
		}
		if c.NTP.Stratum < 1 || c.NTP.Stratum > 15 {
			errs = append(errs, errors.New("ntp.stratum must be between 1 and 15"))
		}
	}

	// Validate NTS config
	if c.NTS.Enabled {
		if c.NTS.CertPath == "" {
			errs = append(errs, errors.New("nts.cert_path is required when NTS is enabled"))
		}
		if c.NTS.KeyPath == "" {
			errs = append(errs, errors.New("nts.key_path is required when NTS is enabled"))
		}
	}

	// Validate PTP config
	if c.PTP.Enabled {
		if c.PTP.Interface == "" {
			errs = append(errs, errors.New("ptp.interface is required when PTP is enabled"))
		}
		if c.PTP.Domain < 0 || c.PTP.Domain > 127 {
			errs = append(errs, errors.New("ptp.domain must be between 0 and 127"))
		}
	}

	// Validate GPSDO config
	if c.GPSDO.Enabled {
		switch c.GPSDO.Type {
		case "serialpps":
			if c.GPSDO.SerialDevice == "" {
				errs = append(errs, errors.New("gpsdo.serial_device is required for serialpps type"))
			}
			if c.GPSDO.PPSDevice == "" {
				errs = append(errs, errors.New("gpsdo.pps_device is required for serialpps type"))
			}
		case "network":
			if c.GPSDO.NetworkAddr == "" {
				errs = append(errs, errors.New("gpsdo.network_address is required for network type"))
			}
		case "dummy":
			// No validation needed
		default:
			errs = append(errs, fmt.Errorf("invalid gpsdo.type: %s", c.GPSDO.Type))
		}
	}

	// Validate Web config
	if c.Web.Port < 1 || c.Web.Port > 65535 {
		errs = append(errs, errors.New("web.port must be between 1 and 65535"))
	}
	if c.Web.TLSEnabled {
		if c.Web.TLSCert == "" {
			errs = append(errs, errors.New("web.tls_cert is required when TLS is enabled"))
		}
		if c.Web.TLSKey == "" {
			errs = append(errs, errors.New("web.tls_key is required when TLS is enabled"))
		}
	}
	if c.Web.JWTSecret == "" && len(c.Web.Users) > 0 {
		errs = append(errs, errors.New("web.jwt_secret is required when users are configured"))
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Reload reloads configuration from the specified path.
func (c *Config) Reload(path string) error {
	newCfg, err := Load(path)
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.NTP = newCfg.NTP
	c.NTS = newCfg.NTS
	c.PTP = newCfg.PTP
	c.GPSDO = newCfg.GPSDO
	c.Web = newCfg.Web
	c.Logging = newCfg.Logging

	return nil
}

// GetNTP returns a copy of the NTP configuration.
func (c *Config) GetNTP() NTPConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.NTP
}

// GetNTS returns a copy of the NTS configuration.
func (c *Config) GetNTS() NTSConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.NTS
}

// GetPTP returns a copy of the PTP configuration.
func (c *Config) GetPTP() PTPConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.PTP
}

// GetGPSDO returns a copy of the GPSDO configuration.
func (c *Config) GetGPSDO() GPSDOConfig {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.GPSDO
}

// ToYAML exports the configuration to YAML format.
func (c *Config) ToYAML() ([]byte, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return yaml.Marshal(c)
}

// SaveToFile writes the configuration to a file.
func (c *Config) SaveToFile(path string) error {
	data, err := c.ToYAML()
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// expandEnvBraces expands only ${VAR} style environment variables.
// This avoids conflicts with bcrypt hashes which contain $ characters.
func expandEnvBraces(s string) string {
	result := s
	for {
		start := -1
		for i := 0; i < len(result)-1; i++ {
			if result[i] == '$' && result[i+1] == '{' {
				start = i
				break
			}
		}
		if start == -1 {
			break
		}

		end := -1
		for i := start + 2; i < len(result); i++ {
			if result[i] == '}' {
				end = i
				break
			}
		}
		if end == -1 {
			break
		}

		varName := result[start+2 : end]
		varValue := os.Getenv(varName)
		result = result[:start] + varValue + result[end+1:]
	}
	return result
}
