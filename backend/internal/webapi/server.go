// Package webapi provides the REST API and WebSocket endpoints for SPTime.
// It exposes status, configuration, and control endpoints for the web GUI.
package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/cors"
	"github.com/sptime/sptime/internal/auth"
	"github.com/sptime/sptime/internal/clock"
	"github.com/sptime/sptime/internal/config"
	"github.com/sptime/sptime/internal/gpsdo"
	"github.com/sptime/sptime/internal/logging"
	"github.com/sptime/sptime/internal/metrics"
	"github.com/sptime/sptime/internal/ntp"
	"github.com/sptime/sptime/internal/nts"
	"github.com/sptime/sptime/internal/ptp"
)

// Services holds references to all service instances.
type Services struct {
	Clock      *clock.Manager
	NTP        *ntp.Server
	NTS        *nts.Server
	PTP        *ptp.Grandmaster
	GPSDO      gpsdo.TimeReference
	Metrics    *metrics.Collector
	Config     *config.Config
	ConfigPath string
}

// Server implements the Web API server.
type Server struct {
	mu sync.RWMutex

	cfg        config.WebConfig
	services   *Services
	log        *logging.Logger
	auth       *auth.Authenticator
	configPath string

	router   *mux.Router
	server   *http.Server
	upgrader websocket.Upgrader

	clients   map[*websocket.Conn]bool
	clientsMu sync.RWMutex
}

// NewServer creates a new Web API server.
func NewServer(cfg config.WebConfig, services *Services, log *logging.Logger) (*Server, error) {
	s := &Server{
		cfg:        cfg,
		services:   services,
		log:        log.WithField("module", "webapi"),
		auth:       auth.NewAuthenticator(cfg),
		configPath: services.ConfigPath,
		clients:    make(map[*websocket.Conn]bool),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				for _, allowed := range cfg.AllowedOrigins {
					if allowed == "*" || allowed == origin {
						return true
					}
				}
				return len(cfg.AllowedOrigins) == 0
			},
		},
	}

	s.setupRoutes()
	return s, nil
}

// setupRoutes configures all API routes.
func (s *Server) setupRoutes() {
	s.router = mux.NewRouter()

	api := s.router.PathPrefix("/api").Subrouter()

	// Public routes
	api.HandleFunc("/health", s.handleHealth).Methods("GET")
	api.HandleFunc("/auth/login", s.handleLogin).Methods("POST")

	// Status routes (read permission)
	api.HandleFunc("/status", s.authMiddleware(s.handleStatus, "read")).Methods("GET")
	api.HandleFunc("/status/clock", s.authMiddleware(s.handleClockStatus, "read")).Methods("GET")
	api.HandleFunc("/status/ntp", s.authMiddleware(s.handleNTPStatus, "read")).Methods("GET")
	api.HandleFunc("/status/nts", s.authMiddleware(s.handleNTSStatus, "read")).Methods("GET")
	api.HandleFunc("/status/ptp", s.authMiddleware(s.handlePTPStatus, "read")).Methods("GET")
	api.HandleFunc("/status/gpsdo", s.authMiddleware(s.handleGPSDOStatus, "read")).Methods("GET")

	// Peers and sources
	api.HandleFunc("/peers", s.authMiddleware(s.handleGetPeers, "read")).Methods("GET")
	api.HandleFunc("/satellites", s.authMiddleware(s.handleSatellites, "read")).Methods("GET")

	// NTP Peer CRUD (config permission)
	api.HandleFunc("/config/ntp/peers", s.authMiddleware(s.handleGetNTPPeers, "config")).Methods("GET")
	api.HandleFunc("/config/ntp/peers", s.authMiddleware(s.handleAddNTPPeer, "config")).Methods("POST")
	api.HandleFunc("/config/ntp/peers/{index}", s.authMiddleware(s.handleUpdateNTPPeer, "config")).Methods("PUT")
	api.HandleFunc("/config/ntp/peers/{index}", s.authMiddleware(s.handleDeleteNTPPeer, "config")).Methods("DELETE")

	// Configuration endpoints
	api.HandleFunc("/config", s.authMiddleware(s.handleGetConfig, "config")).Methods("GET")
	api.HandleFunc("/config/ntp", s.authMiddleware(s.handleUpdateNTPConfig, "config")).Methods("PUT")
	api.HandleFunc("/config/nts", s.authMiddleware(s.handleUpdateNTSConfig, "config")).Methods("PUT")
	api.HandleFunc("/config/ptp", s.authMiddleware(s.handleUpdatePTPConfig, "config")).Methods("PUT")
	api.HandleFunc("/config/gpsdo", s.authMiddleware(s.handleUpdateGPSDOConfig, "config")).Methods("PUT")

	// System info
	api.HandleFunc("/system/interfaces", s.authMiddleware(s.handleGetNetworkInterfaces, "read")).Methods("GET")
	api.HandleFunc("/system/serial-ports", s.authMiddleware(s.handleGetSerialPorts, "read")).Methods("GET")

	// Admin routes
	api.HandleFunc("/admin/reload-config", s.authMiddleware(s.handleReloadConfig, "admin")).Methods("POST")
	api.HandleFunc("/admin/restart-service/{service}", s.authMiddleware(s.handleRestartService, "admin")).Methods("POST")

	// Logs
	api.HandleFunc("/logs", s.authMiddleware(s.handleLogs, "read")).Methods("GET")
	api.HandleFunc("/ws/logs", s.handleWebSocketLogs).Methods("GET")
	api.HandleFunc("/ws/status", s.handleWebSocketStatus).Methods("GET")

	// Prometheus metrics
	s.router.Handle("/metrics", promhttp.Handler())

	// Static files (SPA support)
	s.router.PathPrefix("/").Handler(s.spaHandler())
}

// spaHandler returns a handler that serves static files and falls back to index.html for SPA routing.
func (s *Server) spaHandler() http.Handler {
	staticDir := s.cfg.StaticDir
	fs := http.FileServer(http.Dir(staticDir))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := staticDir + r.URL.Path

		// Check if file exists
		_, err := os.Stat(path)
		if os.IsNotExist(err) {
			// Serve index.html for SPA routing
			http.ServeFile(w, r, staticDir+"/index.html")
			return
		}

		// Serve the static file
		fs.ServeHTTP(w, r)
	})
}

// authMiddleware wraps a handler with authentication and authorization.
func (s *Server) authMiddleware(next http.HandlerFunc, permission string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.ReadOnlyMode && permission == "read" {
			next(w, r)
			return
		}

		token := auth.ExtractToken(r)
		if token == "" {
			s.respondError(w, http.StatusUnauthorized, "missing authentication token")
			return
		}

		claims, err := s.auth.ValidateToken(token)
		if err != nil {
			s.respondError(w, http.StatusUnauthorized, err.Error())
			return
		}

		if !auth.HasPermission(claims.Role, permission) {
			s.respondError(w, http.StatusForbidden, "insufficient permissions")
			return
		}

		ctx := context.WithValue(r.Context(), "user", &auth.User{
			Username: claims.Username,
			Role:     claims.Role,
		})
		next(w, r.WithContext(ctx))
	}
}

// Start starts the Web API server.
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.BindAddress, s.cfg.Port)

	c := cors.New(cors.Options{
		AllowedOrigins:   s.cfg.AllowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
	})

	s.server = &http.Server{
		Addr:         addr,
		Handler:      c.Handler(s.router),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.log.Info("Web API server starting", "address", addr)

	go s.broadcastStatusUpdates()

	if s.cfg.TLSEnabled {
		return s.server.ListenAndServeTLS(s.cfg.TLSCert, s.cfg.TLSKey)
	}
	return s.server.ListenAndServe()
}

// Stop stops the Web API server.
func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	s.clientsMu.Lock()
	for client := range s.clients {
		client.Close()
	}
	s.clientsMu.Unlock()

	return s.server.Shutdown(ctx)
}

// Response helpers
func (s *Server) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func (s *Server) respondError(w http.ResponseWriter, status int, message string) {
	s.respondJSON(w, status, map[string]string{"error": message})
}

func (s *Server) respondSuccess(w http.ResponseWriter, message string) {
	s.respondJSON(w, http.StatusOK, map[string]string{"status": "success", "message": message})
}

// saveConfig saves the config to file with atomic write
func (s *Server) saveConfig() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create temp file in same directory for atomic rename
	dir := filepath.Dir(s.configPath)
	tmpFile, err := os.CreateTemp(dir, "config-*.yaml.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()

	// Write config to temp file
	data, err := s.services.Config.ToYAML()
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write config: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to sync config: %w", err)
	}
	tmpFile.Close()

	// Set proper permissions
	if err := os.Chmod(tmpPath, 0640); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to set permissions: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, s.configPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename config file: %w", err)
	}

	s.log.Info("Configuration saved", "path", s.configPath)
	return nil
}

// === Health & Auth Handlers ===

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "healthy",
		"version": "1.0.0",
		"time":    time.Now().UTC().Format(time.RFC3339Nano),
	})
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token     string `json:"token"`
	ExpiresIn int    `json:"expires_in"`
	User      struct {
		Username string    `json:"username"`
		Role     auth.Role `json:"role"`
	} `json:"user"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	token, err := s.auth.Authenticate(req.Username, req.Password)
	if err != nil {
		s.log.Warn("Login failed", "username", req.Username, "error", err)
		s.respondError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	user := s.cfg.Users[0]
	for _, u := range s.cfg.Users {
		if u.Username == req.Username {
			user = u
			break
		}
	}

	resp := LoginResponse{
		Token:     token,
		ExpiresIn: int(s.cfg.JWTExpiry.Seconds()),
	}
	resp.User.Username = req.Username
	resp.User.Role = auth.Role(user.Role)

	s.log.Info("Login successful", "username", req.Username)
	s.respondJSON(w, http.StatusOK, resp)
}

// === Status Handlers ===

type StatusResponse struct {
	Time   time.Time     `json:"time"`
	Uptime float64       `json:"uptime_seconds"`
	Clock  clock.Status  `json:"clock"`
	NTP    *ntp.Status   `json:"ntp,omitempty"`
	NTS    *nts.Status   `json:"nts,omitempty"`
	PTP    *ptp.Status   `json:"ptp,omitempty"`
	GPSDO  *gpsdo.Status `json:"gpsdo,omitempty"`
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	resp := StatusResponse{
		Time:   time.Now().UTC(),
		Uptime: time.Since(s.services.Metrics.StartTime).Seconds(),
		Clock:  s.services.Clock.GetStatus(),
	}

	if s.services.NTP != nil {
		status := s.services.NTP.GetStatus()
		resp.NTP = &status
	}
	if s.services.NTS != nil {
		status := s.services.NTS.GetStatus()
		resp.NTS = &status
	}
	if s.services.PTP != nil {
		status := s.services.PTP.GetStatus()
		resp.PTP = &status
	}
	if s.services.GPSDO != nil {
		status := s.services.GPSDO.GetStatus()
		resp.GPSDO = &status
	}

	s.respondJSON(w, http.StatusOK, resp)
}

func (s *Server) handleClockStatus(w http.ResponseWriter, r *http.Request) {
	s.respondJSON(w, http.StatusOK, s.services.Clock.GetStatus())
}

func (s *Server) handleNTPStatus(w http.ResponseWriter, r *http.Request) {
	if s.services.NTP == nil {
		s.respondError(w, http.StatusNotFound, "NTP server not enabled")
		return
	}
	s.respondJSON(w, http.StatusOK, s.services.NTP.GetStatus())
}

func (s *Server) handleNTSStatus(w http.ResponseWriter, r *http.Request) {
	if s.services.NTS == nil {
		s.respondError(w, http.StatusNotFound, "NTS server not enabled")
		return
	}
	s.respondJSON(w, http.StatusOK, s.services.NTS.GetStatus())
}

func (s *Server) handlePTPStatus(w http.ResponseWriter, r *http.Request) {
	if s.services.PTP == nil {
		s.respondError(w, http.StatusNotFound, "PTP not enabled")
		return
	}
	s.respondJSON(w, http.StatusOK, s.services.PTP.GetStatus())
}

func (s *Server) handleGPSDOStatus(w http.ResponseWriter, r *http.Request) {
	if s.services.GPSDO == nil {
		s.respondError(w, http.StatusNotFound, "GPSDO not enabled")
		return
	}
	s.respondJSON(w, http.StatusOK, s.services.GPSDO.GetStatus())
}

func (s *Server) handleGetPeers(w http.ResponseWriter, r *http.Request) {
	if s.services.NTP == nil {
		s.respondJSON(w, http.StatusOK, []ntp.PeerStatus{})
		return
	}
	s.respondJSON(w, http.StatusOK, s.services.NTP.GetPeers())
}

func (s *Server) handleSatellites(w http.ResponseWriter, r *http.Request) {
	if s.services.GPSDO == nil {
		s.respondJSON(w, http.StatusOK, []gpsdo.Satellite{})
		return
	}
	s.respondJSON(w, http.StatusOK, s.services.GPSDO.GetSatellites())
}

// === Configuration Handlers ===

type FullConfigResponse struct {
	NTP   config.NTPConfig   `json:"ntp"`
	NTS   NTSConfigResponse  `json:"nts"`
	PTP   config.PTPConfig   `json:"ptp"`
	GPSDO config.GPSDOConfig `json:"gpsdo"`
}

type NTSConfigResponse struct {
	Enabled   bool   `json:"enabled"`
	KEPort    int    `json:"ke_port"`
	CertPath  string `json:"cert_path"`
	KeyPath   string `json:"key_path"`
	MinTLSVer string `json:"min_tls_version"`
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	cfg := FullConfigResponse{
		NTP:   s.services.Config.GetNTP(),
		PTP:   s.services.Config.GetPTP(),
		GPSDO: s.services.Config.GetGPSDO(),
	}
	ntsCfg := s.services.Config.GetNTS()
	cfg.NTS = NTSConfigResponse{
		Enabled:   ntsCfg.Enabled,
		KEPort:    ntsCfg.KEPort,
		CertPath:  ntsCfg.CertPath,
		KeyPath:   ntsCfg.KeyPath,
		MinTLSVer: ntsCfg.MinTLSVer,
	}
	s.respondJSON(w, http.StatusOK, cfg)
}

// === NTP Peer CRUD ===

func (s *Server) handleGetNTPPeers(w http.ResponseWriter, r *http.Request) {
	s.respondJSON(w, http.StatusOK, s.services.Config.GetNTP().UpstreamPeers)
}

type AddPeerRequest struct {
	Address    string `json:"address"`
	Prefer     bool   `json:"prefer"`
	IBurst     bool   `json:"iburst"`
	MinPoll    int    `json:"minpoll"`
	MaxPoll    int    `json:"maxpoll"`
	NTSEnabled bool   `json:"nts_enabled"`
}

func (s *Server) handleAddNTPPeer(w http.ResponseWriter, r *http.Request) {
	var req AddPeerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Address == "" {
		s.respondError(w, http.StatusBadRequest, "address is required")
		return
	}

	// Validate address format - try to parse as host:port
	host, port, err := net.SplitHostPort(req.Address)
	if err != nil {
		// No port specified, add default NTP port
		host = req.Address
		port = "123"
		req.Address = net.JoinHostPort(host, port)
	}

	// Verify host is resolvable
	if _, err := net.LookupHost(host); err != nil {
		s.respondError(w, http.StatusBadRequest, "cannot resolve hostname: "+host)
		return
	}

	// Set defaults
	if req.MinPoll == 0 {
		req.MinPoll = 4
	}
	if req.MaxPoll == 0 {
		req.MaxPoll = 6
	}

	peer := config.PeerConfig{
		Address:    req.Address,
		Prefer:     req.Prefer,
		IBurst:     req.IBurst,
		MinPoll:    req.MinPoll,
		MaxPoll:    req.MaxPoll,
		NTSEnabled: req.NTSEnabled,
	}

	s.services.Config.NTP.UpstreamPeers = append(s.services.Config.NTP.UpstreamPeers, peer)

	if err := s.saveConfig(); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}

	s.log.Info("NTP peer added", "address", req.Address)
	s.respondJSON(w, http.StatusCreated, peer)
}

func (s *Server) handleUpdateNTPPeer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	indexStr := vars["index"]
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid peer index")
		return
	}

	if index < 0 || index >= len(s.services.Config.NTP.UpstreamPeers) {
		s.respondError(w, http.StatusNotFound, "peer not found")
		return
	}

	var req AddPeerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Address == "" {
		s.respondError(w, http.StatusBadRequest, "address is required")
		return
	}

	s.services.Config.NTP.UpstreamPeers[index] = config.PeerConfig{
		Address:    req.Address,
		Prefer:     req.Prefer,
		IBurst:     req.IBurst,
		MinPoll:    req.MinPoll,
		MaxPoll:    req.MaxPoll,
		NTSEnabled: req.NTSEnabled,
	}

	if err := s.saveConfig(); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}

	s.log.Info("NTP peer updated", "index", index, "address", req.Address)
	s.respondSuccess(w, "peer updated")
}

func (s *Server) handleDeleteNTPPeer(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	indexStr := vars["index"]
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid peer index")
		return
	}

	peers := s.services.Config.NTP.UpstreamPeers
	if index < 0 || index >= len(peers) {
		s.respondError(w, http.StatusNotFound, "peer not found")
		return
	}

	addr := peers[index].Address
	s.services.Config.NTP.UpstreamPeers = append(peers[:index], peers[index+1:]...)

	if err := s.saveConfig(); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}

	s.log.Info("NTP peer deleted", "index", index, "address", addr)
	s.respondSuccess(w, "peer deleted")
}

// === Section Config Updates ===

type UpdateNTPConfigRequest struct {
	Enabled      *bool   `json:"enabled,omitempty"`
	Port         *int    `json:"port,omitempty"`
	Stratum      *int    `json:"stratum,omitempty"`
	RefID        *string `json:"ref_id,omitempty"`
	PollInterval *string `json:"poll_interval,omitempty"`
}

func (s *Server) handleUpdateNTPConfig(w http.ResponseWriter, r *http.Request) {
	var req UpdateNTPConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Enabled != nil {
		s.services.Config.NTP.Enabled = *req.Enabled
	}
	if req.Port != nil {
		if *req.Port < 1 || *req.Port > 65535 {
			s.respondError(w, http.StatusBadRequest, "port must be 1-65535")
			return
		}
		s.services.Config.NTP.Port = *req.Port
	}
	if req.Stratum != nil {
		if *req.Stratum < 1 || *req.Stratum > 15 {
			s.respondError(w, http.StatusBadRequest, "stratum must be 1-15")
			return
		}
		s.services.Config.NTP.Stratum = *req.Stratum
	}
	if req.RefID != nil {
		if len(*req.RefID) > 4 {
			s.respondError(w, http.StatusBadRequest, "ref_id must be max 4 characters")
			return
		}
		s.services.Config.NTP.RefID = *req.RefID
	}
	if req.PollInterval != nil {
		dur, err := time.ParseDuration(*req.PollInterval)
		if err != nil {
			s.respondError(w, http.StatusBadRequest, "invalid poll_interval format")
			return
		}
		s.services.Config.NTP.PollInterval = dur
	}

	if err := s.saveConfig(); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}

	s.log.Info("NTP config updated")
	s.respondSuccess(w, "NTP configuration updated")
}

type UpdateNTSConfigRequest struct {
	Enabled   *bool   `json:"enabled,omitempty"`
	KEPort    *int    `json:"ke_port,omitempty"`
	CertPath  *string `json:"cert_path,omitempty"`
	KeyPath   *string `json:"key_path,omitempty"`
	MinTLSVer *string `json:"min_tls_version,omitempty"`
}

func (s *Server) handleUpdateNTSConfig(w http.ResponseWriter, r *http.Request) {
	var req UpdateNTSConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Enabled != nil {
		s.services.Config.NTS.Enabled = *req.Enabled
	}
	if req.KEPort != nil {
		s.services.Config.NTS.KEPort = *req.KEPort
	}
	if req.CertPath != nil {
		s.services.Config.NTS.CertPath = *req.CertPath
	}
	if req.KeyPath != nil {
		s.services.Config.NTS.KeyPath = *req.KeyPath
	}
	if req.MinTLSVer != nil {
		s.services.Config.NTS.MinTLSVer = *req.MinTLSVer
	}

	if err := s.saveConfig(); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}

	s.log.Info("NTS config updated")
	s.respondSuccess(w, "NTS configuration updated")
}

type UpdatePTPConfigRequest struct {
	Enabled        *bool   `json:"enabled,omitempty"`
	Interface      *string `json:"interface,omitempty"`
	Domain         *int    `json:"domain,omitempty"`
	Priority1      *int    `json:"priority1,omitempty"`
	Priority2      *int    `json:"priority2,omitempty"`
	ClockClass     *int    `json:"clock_class,omitempty"`
	TransportMode  *string `json:"transport_mode,omitempty"`
	DelayMechanism *string `json:"delay_mechanism,omitempty"`
	Profile        *string `json:"profile,omitempty"`
	TwoStepFlag    *bool   `json:"two_step_flag,omitempty"`
	LogAnnounceInt *int    `json:"log_announce_interval,omitempty"`
	LogSyncInt     *int    `json:"log_sync_interval,omitempty"`
}

func (s *Server) handleUpdatePTPConfig(w http.ResponseWriter, r *http.Request) {
	var req UpdatePTPConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Enabled != nil {
		s.services.Config.PTP.Enabled = *req.Enabled
	}
	if req.Interface != nil {
		s.services.Config.PTP.Interface = *req.Interface
	}
	if req.Domain != nil {
		if *req.Domain < 0 || *req.Domain > 127 {
			s.respondError(w, http.StatusBadRequest, "domain must be 0-127")
			return
		}
		s.services.Config.PTP.Domain = *req.Domain
	}
	if req.Priority1 != nil {
		s.services.Config.PTP.Priority1 = *req.Priority1
	}
	if req.Priority2 != nil {
		s.services.Config.PTP.Priority2 = *req.Priority2
	}
	if req.ClockClass != nil {
		s.services.Config.PTP.ClockClass = *req.ClockClass
	}
	if req.TransportMode != nil {
		s.services.Config.PTP.TransportMode = *req.TransportMode
	}
	if req.DelayMechanism != nil {
		s.services.Config.PTP.DelayMechanism = *req.DelayMechanism
	}
	if req.Profile != nil {
		s.services.Config.PTP.Profile = *req.Profile
	}
	if req.TwoStepFlag != nil {
		s.services.Config.PTP.TwoStepFlag = *req.TwoStepFlag
	}
	if req.LogAnnounceInt != nil {
		s.services.Config.PTP.LogAnnounceInt = *req.LogAnnounceInt
	}
	if req.LogSyncInt != nil {
		s.services.Config.PTP.LogSyncInt = *req.LogSyncInt
	}

	if err := s.saveConfig(); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}

	s.log.Info("PTP config updated")
	s.respondSuccess(w, "PTP configuration updated")
}

type UpdateGPSDOConfigRequest struct {
	Enabled      *bool   `json:"enabled,omitempty"`
	Type         *string `json:"type,omitempty"`
	SerialDevice *string `json:"serial_device,omitempty"`
	SerialBaud   *int    `json:"serial_baud,omitempty"`
	PPSDevice    *string `json:"pps_device,omitempty"`
	NetworkAddr  *string `json:"network_address,omitempty"`
}

func (s *Server) handleUpdateGPSDOConfig(w http.ResponseWriter, r *http.Request) {
	var req UpdateGPSDOConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Enabled != nil {
		s.services.Config.GPSDO.Enabled = *req.Enabled
	}
	if req.Type != nil {
		validTypes := map[string]bool{"dummy": true, "serialpps": true, "network": true}
		if !validTypes[*req.Type] {
			s.respondError(w, http.StatusBadRequest, "type must be dummy, serialpps, or network")
			return
		}
		s.services.Config.GPSDO.Type = *req.Type
	}
	if req.SerialDevice != nil {
		s.services.Config.GPSDO.SerialDevice = *req.SerialDevice
	}
	if req.SerialBaud != nil {
		s.services.Config.GPSDO.SerialBaud = *req.SerialBaud
	}
	if req.PPSDevice != nil {
		s.services.Config.GPSDO.PPSDevice = *req.PPSDevice
	}
	if req.NetworkAddr != nil {
		s.services.Config.GPSDO.NetworkAddr = *req.NetworkAddr
	}

	if err := s.saveConfig(); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}

	s.log.Info("GPSDO config updated")
	s.respondSuccess(w, "GPSDO configuration updated")
}

// === System Info Handlers ===

type NetworkInterface struct {
	Name         string   `json:"name"`
	MAC          string   `json:"mac"`
	IPs          []string `json:"ips"`
	Up           bool     `json:"up"`
	Loopback     bool     `json:"loopback"`
	PointToPoint bool     `json:"point_to_point"`
}

func (s *Server) handleGetNetworkInterfaces(w http.ResponseWriter, r *http.Request) {
	ifaces, err := net.Interfaces()
	if err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to get interfaces")
		return
	}

	result := make([]NetworkInterface, 0, len(ifaces))
	for _, iface := range ifaces {
		ni := NetworkInterface{
			Name:         iface.Name,
			MAC:          iface.HardwareAddr.String(),
			Up:           iface.Flags&net.FlagUp != 0,
			Loopback:     iface.Flags&net.FlagLoopback != 0,
			PointToPoint: iface.Flags&net.FlagPointToPoint != 0,
		}

		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			ni.IPs = append(ni.IPs, addr.String())
		}

		// Skip loopback for PTP
		if !ni.Loopback {
			result = append(result, ni)
		}
	}

	s.respondJSON(w, http.StatusOK, result)
}

type SerialPort struct {
	Path string `json:"path"`
	Name string `json:"name"`
}

func (s *Server) handleGetSerialPorts(w http.ResponseWriter, r *http.Request) {
	ports := []SerialPort{}

	// Common serial device paths
	patterns := []string{
		"/dev/ttyUSB*",
		"/dev/ttyACM*",
		"/dev/ttyS*",
		"/dev/serial/by-id/*",
	}

	for _, pattern := range patterns {
		matches, _ := filepath.Glob(pattern)
		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil {
				continue
			}
			if info.Mode()&os.ModeDevice != 0 || info.Mode()&os.ModeSymlink != 0 {
				ports = append(ports, SerialPort{
					Path: match,
					Name: filepath.Base(match),
				})
			}
		}
	}

	// Also check for PPS devices
	ppsMatches, _ := filepath.Glob("/dev/pps*")
	for _, match := range ppsMatches {
		ports = append(ports, SerialPort{
			Path: match,
			Name: filepath.Base(match) + " (PPS)",
		})
	}

	s.respondJSON(w, http.StatusOK, ports)
}

// === Admin Handlers ===

func (s *Server) handleReloadConfig(w http.ResponseWriter, r *http.Request) {
	if err := s.services.Config.Reload(s.configPath); err != nil {
		s.respondError(w, http.StatusInternalServerError, "failed to reload config: "+err.Error())
		return
	}

	s.log.Info("Configuration reloaded from disk")
	s.respondSuccess(w, "configuration reloaded")
}

func (s *Server) handleRestartService(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	service := vars["service"]

	switch service {
	case "ntp", "nts", "ptp", "gpsdo":
		// Service restart requires process restart - not supported via API
		// Use systemctl restart sptime or docker restart instead
		s.log.Warn("Service restart requested but not supported", "service", service)
		s.respondError(w, http.StatusNotImplemented,
			"runtime service restart not supported - restart the process using systemctl or docker")
	default:
		s.respondError(w, http.StatusBadRequest, "unknown service: "+service)
	}
}

// === Logs ===

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	level := r.URL.Query().Get("level")
	logs := logging.GetRecentLogs(limit)

	if level != "" {
		filtered := make([]logging.LogEntry, 0)
		for _, log := range logs {
			if log.Level == level {
				filtered = append(filtered, log)
			}
		}
		logs = filtered
	}

	s.respondJSON(w, http.StatusOK, logs)
}

// === WebSocket Handlers ===

func (s *Server) handleWebSocketStatus(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token != "" {
		if _, err := s.auth.ValidateToken(token); err != nil {
			s.respondError(w, http.StatusUnauthorized, "invalid token")
			return
		}
	} else if !s.cfg.ReadOnlyMode {
		s.respondError(w, http.StatusUnauthorized, "authentication required")
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.log.Error("WebSocket upgrade failed", "error", err)
		return
	}

	s.clientsMu.Lock()
	s.clients[conn] = true
	s.clientsMu.Unlock()

	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, conn)
		s.clientsMu.Unlock()
		conn.Close()
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

func (s *Server) handleWebSocketLogs(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.log.Error("WebSocket upgrade failed", "error", err)
		return
	}
	defer conn.Close()

	lastSent := 0
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			logs := logging.GetRecentLogs(50)
			if len(logs) != lastSent {
				if err := conn.WriteJSON(logs); err != nil {
					return
				}
				lastSent = len(logs)
			}
		}
	}
}

func (s *Server) broadcastStatusUpdates() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		s.clientsMu.RLock()
		if len(s.clients) == 0 {
			s.clientsMu.RUnlock()
			continue
		}

		status := StatusResponse{
			Time:   time.Now().UTC(),
			Uptime: time.Since(s.services.Metrics.StartTime).Seconds(),
			Clock:  s.services.Clock.GetStatus(),
		}

		if s.services.NTP != nil {
			ntpStatus := s.services.NTP.GetStatus()
			status.NTP = &ntpStatus
		}
		if s.services.NTS != nil {
			ntsStatus := s.services.NTS.GetStatus()
			status.NTS = &ntsStatus
		}
		if s.services.PTP != nil {
			ptpStatus := s.services.PTP.GetStatus()
			status.PTP = &ptpStatus
		}
		if s.services.GPSDO != nil {
			gpsdoStatus := s.services.GPSDO.GetStatus()
			status.GPSDO = &gpsdoStatus
		}

		deadClients := make([]*websocket.Conn, 0)
		for client := range s.clients {
			if err := client.WriteJSON(status); err != nil {
				deadClients = append(deadClients, client)
			}
		}
		s.clientsMu.RUnlock()

		if len(deadClients) > 0 {
			s.clientsMu.Lock()
			for _, client := range deadClients {
				client.Close()
				delete(s.clients, client)
			}
			s.clientsMu.Unlock()
		}
	}
}
