package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	attendancepkg "github.com/ghalitsar-coder/backend-ifter/internal/attendance"
	"github.com/ghalitsar-coder/backend-ifter/internal/auth"
	"github.com/ghalitsar-coder/backend-ifter/internal/config"
	disciplinepkg "github.com/ghalitsar-coder/backend-ifter/internal/discipline"
	importservice "github.com/ghalitsar-coder/backend-ifter/internal/imports"
	"github.com/ghalitsar-coder/backend-ifter/internal/maintenance"
	"github.com/ghalitsar-coder/backend-ifter/internal/masterdata"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/database"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/logger"
	"github.com/ghalitsar-coder/backend-ifter/internal/platform/metrics"
	"github.com/ghalitsar-coder/backend-ifter/internal/reporting"
	httptransport "github.com/ghalitsar-coder/backend-ifter/internal/transport/http"
)

func Run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log := logger.New(cfg.Log.Level, cfg.Log.Format)
	slog.SetDefault(log)

	pool, err := database.NewPool(ctx, cfg.DB)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()
	metrics.RegisterDBPool(pool)

	tokens := auth.NewTokenManager(cfg.Auth.JWTSecret, cfg.Auth.JWTIssuer, cfg.Auth.AccessTokenTTL)
	authService := auth.NewService(pool, tokens, auth.ServiceConfig{
		RefreshTokenTTL: cfg.Auth.RefreshTokenTTL,
		InviteTokenTTL:  cfg.Auth.InviteTokenTTL,
		ResetTokenTTL:   cfg.Auth.ResetTokenTTL,
		BcryptCost:      cfg.Auth.BcryptCost,
	}, log)
	if err := authService.EnsureBootstrapAdmin(ctx, auth.BootstrapAdmin{
		Username: cfg.Auth.BootstrapAdmin.Username,
		Email:    cfg.Auth.BootstrapAdmin.Email,
		Password: cfg.Auth.BootstrapAdmin.Password,
		FullName: cfg.Auth.BootstrapAdmin.FullName,
	}); err != nil {
		return fmt.Errorf("ensure bootstrap administrator: %w", err)
	}

	masterDataService := masterdata.NewService(pool, log)
	importService, err := importservice.NewService(pool, log, importservice.Config{
		Dir:            cfg.Imports.Dir,
		MaxFileSize:    cfg.Imports.MaxFileSize,
		MaxRows:        cfg.Imports.MaxRows,
		FileRetention:  cfg.Imports.FileRetention,
		ErrorRetention: cfg.Imports.ErrorRetention,
	})
	if err != nil {
		return fmt.Errorf("create import service: %w", err)
	}
	go importService.RunCleanup(ctx)

	attendanceService := attendancepkg.NewService(pool, log)
	go attendanceService.RunAutoClose(ctx)

	disciplineService := disciplinepkg.NewService(pool, log)
	reportingService := reporting.NewService(pool, log)

	maintenanceService := maintenance.NewService(pool, log, maintenance.Config{
		AuditLogRetention:     cfg.Maintenance.AuditLogRetention,
		RefreshTokenRetention: cfg.Maintenance.RefreshTokenRetention,
		Interval:              cfg.Maintenance.Interval,
	})
	go maintenanceService.Run(ctx)

	listener, err := net.Listen("tcp", cfg.HTTP.Addr())
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.HTTP.Addr(), err)
	}

	server := &http.Server{
		Handler:           httptransport.New(log, pool, authService, masterDataService, importService, attendanceService, disciplineService, reportingService, cfg.Auth, cfg.HTTP.TrustedProxies, httptransport.WithHardening(cfg.Hardening)),
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Serve(listener)
	}()
	log.Info("server_started", "addr", cfg.HTTP.Addr(), "env", cfg.App.Env)

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown server: %w", err)
		}
		log.Info("server_stopped")
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve: %w", err)
	}
}
