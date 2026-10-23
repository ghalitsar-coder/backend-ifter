package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/ghalitsar-coder/backend-ifter/internal/auth"
	"github.com/ghalitsar-coder/backend-ifter/internal/config"
	"github.com/ghalitsar-coder/backend-ifter/internal/item"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/logger"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/metrics"
	httptransport "github.com/ghalitsar-coder/backend-ifter/internal/transport/http"
)

// Version is injected at build time:
// go build -ldflags "-X .../internal/app.Version=1.2.3".
var Version = "dev"

// Run is the composition root: build dependencies, serve, then shut down
// gracefully when ctx is cancelled.
func Run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log := logger.New(cfg.Log.Level, cfg.Log.Format)
	slog.SetDefault(log)
	log.Info("app_starting", "name", cfg.App.Name, "env", cfg.App.Env, "version", Version)

	pool, err := database.NewPool(ctx, cfg.DB)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	m := metrics.New()
	m.RegisterDBPool(pool)

	itemService := item.NewService(item.NewPostgresRepository(pool))
	tokens := auth.NewTokenService(cfg.Auth)

	handler := httptransport.New(httptransport.Deps{
		Config:    cfg,
		Logger:    log,
		Metrics:   m,
		Readiness: pool,
		Tokens:    tokens,
		Items:     itemService,
		Version:   Version,
	})

	server := &http.Server{
		Addr:              cfg.HTTP.Addr(),
		Handler:           handler,
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("http_server_started", "addr", server.Addr)
		serverErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serverErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	case <-ctx.Done():
		log.Info("shutdown_started")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown http server: %w", err)
	}

	log.Info("shutdown_complete")
	return nil
}
