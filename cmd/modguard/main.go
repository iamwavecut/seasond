package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iamwavecut/seasond/internal/config"
	"github.com/iamwavecut/seasond/internal/httpapi"
	"github.com/iamwavecut/seasond/internal/indexsync"
	"github.com/iamwavecut/seasond/internal/storage"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}

	level := new(slog.LevelVar)
	level.Set(cfg.LogLevel)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	store, err := storage.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer store.Close()

	httpClient := &http.Client{Timeout: cfg.HTTPTimeout}
	syncer := indexsync.New(store, indexsync.Config{
		IndexBase:      cfg.IndexBase,
		BootstrapSince: cfg.BootstrapSince,
		HTTPClient:     httpClient,
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go runIndexLoop(ctx, logger, syncer, cfg.PollInterval)

	handler := httpapi.NewHandler(httpapi.Config{
		Store:                 store,
		UpstreamBase:          cfg.UpstreamProxyBase,
		DefaultMinAge:         cfg.DefaultMinAge,
		HTTPClient:            httpClient,
		Logger:                logger,
		AllowRedirectsForInfo: cfg.AllowRedirectsForInfo,
	})
	server := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: handler,
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTPTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown failed", "error", err)
		}
	}()

	logger.Info("listening", "addr", cfg.ListenAddr)
	err = server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func runIndexLoop(ctx context.Context, logger *slog.Logger, syncer *indexsync.Syncer, interval time.Duration) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			stats, err := syncer.SyncOnce(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("index sync failed", "error", err)
			} else if err == nil {
				logger.Info("index sync complete", "rows", stats.Rows)
			}
			timer.Reset(interval)
		}
	}
}
