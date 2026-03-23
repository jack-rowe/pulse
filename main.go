package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jack-rowe/pulse/api"
	"github.com/jack-rowe/pulse/checker"
	"github.com/jack-rowe/pulse/config"
	"github.com/jack-rowe/pulse/notifier"
	"github.com/jack-rowe/pulse/scheduler"
	"github.com/jack-rowe/pulse/store"
)

// Set at build time via -ldflags.
var version = "dev"

func main() {
	os.Exit(run())
}

func run() int {
	// CLI flags
	configPath := flag.String("config", "config.yaml", "Path to config file")
	initConfig := flag.Bool("init", false, "Generate a default config.yaml and exit")
	validateOnly := flag.Bool("validate", false, "Validate config and exit")
	showVersion := flag.Bool("version", false, "Print version and exit")
	debug := flag.Bool("debug", false, "Enable debug logging")
	flag.Parse()

	if *showVersion {
		fmt.Printf("pulse %s\n", version)
		return 0
	}

	// --init: write example config
	if *initConfig {
		if err := os.WriteFile("config.yaml", []byte(config.GenerateDefault()), 0644); err != nil {
			slog.Error("failed to write config", "error", err)
			return 1
		}
		fmt.Println("Created config.yaml - edit it with your endpoints and run again.")
		return 0
	}

	// Set up structured logging
	logLevel := slog.LevelInfo
	if *debug {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})))

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("config error", "error", err)
		return 1
	}

	if *validateOnly {
		fmt.Printf("Config OK - %d endpoints defined\n", len(cfg.Endpoints))
		return 0
	}

	slog.Info("starting pulse", "version", version, "endpoints", len(cfg.Endpoints))

	// Init store
	db, err := store.NewBolt(cfg.Storage.Path)
	if err != nil {
		slog.Error("failed to init store", "error", err)
		return 1
	}
	defer db.Close()

	// Init notifiers
	var notifiers []notifier.Notifier
	notifiers = append(notifiers, notifier.NewLog())
	if cfg.Alerting.Slack.WebhookURL != "" {
		notifiers = append(notifiers, notifier.NewSlack(cfg.Alerting.Slack.WebhookURL))
	}
	if cfg.Alerting.Discord.WebhookURL != "" {
		notifiers = append(notifiers, notifier.NewDiscord(cfg.Alerting.Discord.WebhookURL))
	}
	if cfg.Alerting.Webhook.URL != "" {
		notifiers = append(notifiers, notifier.NewWebhook(cfg.Alerting.Webhook.URL, cfg.Alerting.Webhook.Headers))
	}
	if cfg.Alerting.SMTP.Host != "" {
		notifiers = append(notifiers, notifier.NewSMTP(
			cfg.Alerting.SMTP.Host,
			cfg.Alerting.SMTP.Port,
			cfg.Alerting.SMTP.Username,
			cfg.Alerting.SMTP.Password,
			cfg.Alerting.SMTP.From,
			cfg.Alerting.SMTP.To,
		))
	}
	alert := notifier.NewMulti(notifiers...)

	// Build checkers from config
	var targets []scheduler.Target
	for _, ep := range cfg.Endpoints {
		var chk checker.Checker
		switch ep.Type {
		case "http":
			chk = checker.NewHTTP(ep.URL, ep.Method, ep.ExpectedStatus, ep.ExpectedBody, ep.Headers, ep.Timeout())
		case "tcp":
			chk = checker.NewTCP(ep.Address, ep.Timeout())
		case "websocket":
			chk = checker.NewWebSocket(ep.URL, ep.Timeout())
		default:
			slog.Error("unknown check type", "endpoint", ep.Name, "type", ep.Type)
			return 1
		}
		targets = append(targets, scheduler.Target{
			Name:          ep.Name,
			Interval:      ep.Interval(),
			FailThreshold: ep.FailThreshold,
			Checker:       chk,
		})
		slog.Info("registered endpoint", "name", ep.Name, "type", ep.Type, "interval", ep.Interval())
	}

	// Start scheduler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sched := scheduler.New(db, alert)
	sched.StartAsync(ctx, targets)

	// Start data retention cleanup
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cutoff := time.Now().Add(-time.Duration(cfg.Storage.RetentionDays) * 24 * time.Hour)
				deleted, err := db.PurgeOlderThan(cutoff)
				if err != nil {
					slog.Error("purge failed", "error", err)
				} else if deleted > 0 {
					slog.Info("purged old records", "deleted", deleted)
				}
			}
		}
	}()

	// Start API server
	handler := api.NewServer(db, cfg.Endpoints, cfg.Server.APIKey)
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	serverErr := make(chan error, 1)

	go func() {
		slog.Info("status API listening", "addr", addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverErr <- err
		}
	}()

	// Block until SIGINT/SIGTERM or server failure.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sig)

	exitCode := 0
	select {
	case <-sig:
		slog.Info("shutting down")
	case err := <-serverErr:
		slog.Error("server error", "error", err)
		exitCode = 1
	}
	cancel()

	// Graceful HTTP shutdown with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil && err != http.ErrServerClosed {
		slog.Error("http shutdown error", "error", err)
		exitCode = 1
	}

	return exitCode
}
