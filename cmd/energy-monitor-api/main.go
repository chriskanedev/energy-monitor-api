package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chriskanedev/energy-monitor-api/internal/config"
	"github.com/chriskanedev/energy-monitor-api/internal/httpapi"
	"github.com/chriskanedev/energy-monitor-api/internal/poller"
	"github.com/chriskanedev/energy-monitor-api/internal/shelly"
	"github.com/chriskanedev/energy-monitor-api/internal/store"
	"github.com/joho/godotenv"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("service stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	_ = godotenv.Load()

	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config/energy-monitor.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	db, err := store.Open(cfg.Storage.SQLitePath)
	if err != nil {
		return err
	}
	defer db.Close()

	apiPoller := poller.New(cfg, shelly.NewClient(http.DefaultClient), db, logger)
	if home, grid, err := db.RecentHistory(context.Background(), 24); err == nil {
		apiPoller.SeedHistory(home, grid)
	} else {
		logger.Warn("failed to seed history", "error", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go apiPoller.Run(ctx)

	server := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           httpapi.New(apiPoller, cfg.Server.CORSAllowedOrigins).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("energy monitor api listening", "addr", cfg.Server.Addr)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			if errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			return err
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
