// Package main is the entry point for the SPTime server.
// SPTime provides NTP, NTS (Network Time Security), and PTP (Precision Time Protocol)
// services with optional GPSDO integration for stratum 1 operation.
package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/sptime/sptime/internal/clock"
	"github.com/sptime/sptime/internal/config"
	"github.com/sptime/sptime/internal/gpsdo"
	"github.com/sptime/sptime/internal/logging"
	"github.com/sptime/sptime/internal/metrics"
	"github.com/sptime/sptime/internal/ntp"
	"github.com/sptime/sptime/internal/nts"
	"github.com/sptime/sptime/internal/ptp"
	"github.com/sptime/sptime/internal/webapi"
)

var (
	configPath = flag.String("config", "/etc/sptime/config.yaml", "Path to configuration file")
	version    = "1.0.0"
)

func main() {
	flag.Parse()

	// Initialise logging first
	log := logging.New("sptime")
	log.Info("SPTime starting", "version", version)

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Configure log level from config
	logging.SetLevel(cfg.Logging.Level)
	log.Info("Configuration loaded", "path", *configPath)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialise metrics collector
	metricsCollector := metrics.New()

	// Initialise clock discipline manager
	clockMgr := clock.NewManager(log, metricsCollector)

	// Initialise GPSDO if configured
	var gpsdoRef gpsdo.TimeReference
	if cfg.GPSDO.Enabled {
		gpsdoRef, err = gpsdo.New(cfg.GPSDO, log)
		if err != nil {
			log.Error("Failed to initialize GPSDO", "error", err)
			// Continue without GPSDO - fall back to NTP only
		} else {
			clockMgr.SetPrimaryReference(gpsdoRef)
			log.Info("GPSDO initialized", "type", cfg.GPSDO.Type)
		}
	}

	// Initialise NTP server
	ntpServer, err := ntp.NewServer(cfg.NTP, clockMgr, log, metricsCollector)
	if err != nil {
		log.Error("Failed to initialize NTP server", "error", err)
		os.Exit(1)
	}

	// Initialise NTS server if enabled
	var ntsServer *nts.Server
	if cfg.NTS.Enabled {
		ntsServer, err = nts.NewServer(cfg.NTS, clockMgr, log, metricsCollector)
		if err != nil {
			log.Error("Failed to initialize NTS server", "error", err)
			os.Exit(1)
		}
		log.Info("NTS server initialized")
	}

	// Initialise PTP Grandmaster if enabled
	var ptpGM *ptp.Grandmaster
	if cfg.PTP.Enabled {
		ptpGM, err = ptp.NewGrandmaster(cfg.PTP, clockMgr, log, metricsCollector)
		if err != nil {
			log.Error("Failed to initialize PTP Grandmaster", "error", err)
			os.Exit(1)
		}
		log.Info("PTP Grandmaster initialized", "domain", cfg.PTP.Domain)
	}

	// Create service registry for web API
	services := &webapi.Services{
		Clock:      clockMgr,
		NTP:        ntpServer,
		NTS:        ntsServer,
		PTP:        ptpGM,
		GPSDO:      gpsdoRef,
		Metrics:    metricsCollector,
		Config:     cfg,
		ConfigPath: *configPath,
	}

	// Initialise Web API server
	apiServer, err := webapi.NewServer(cfg.Web, services, log)
	if err != nil {
		log.Error("Failed to initialize Web API server", "error", err)
		os.Exit(1)
	}

	// Start all services
	errChan := make(chan error, 5)

	go func() {
		if err := ntpServer.Start(ctx); err != nil {
			errChan <- err
		}
	}()
	log.Info("NTP server started", "port", cfg.NTP.Port)

	if ntsServer != nil {
		go func() {
			if err := ntsServer.Start(ctx); err != nil {
				errChan <- err
			}
		}()
		log.Info("NTS server started", "ke_port", cfg.NTS.KEPort)
	}

	if ptpGM != nil {
		go func() {
			if err := ptpGM.Start(ctx); err != nil {
				errChan <- err
			}
		}()
		log.Info("PTP Grandmaster started", "interface", cfg.PTP.Interface)
	}

	if gpsdoRef != nil {
		go func() {
			if err := gpsdoRef.Start(ctx); err != nil {
				errChan <- err
			}
		}()
	}

	go func() {
		if err := apiServer.Start(); err != nil {
			errChan <- err
		}
	}()
	log.Info("Web API server started", "port", cfg.Web.Port)

	// Start clock discipline loop
	go clockMgr.Run(ctx)

	// Wait for shutdown signal or error
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		log.Info("Received shutdown signal", "signal", sig)
	case err := <-errChan:
		log.Error("Service error", "error", err)
	}

	// Graceful shutdown
	log.Info("Initiating graceful shutdown...")
	cancel()

	// Stop services in reverse order
	if err := apiServer.Stop(); err != nil {
		log.Error("Error stopping Web API", "error", err)
	}
	if ptpGM != nil {
		ptpGM.Stop()
	}
	if ntsServer != nil {
		ntsServer.Stop()
	}
	ntpServer.Stop()
	if gpsdoRef != nil {
		gpsdoRef.Stop()
	}

	log.Info("SPTime shutdown complete")
}
