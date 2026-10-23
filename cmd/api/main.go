package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/ghalitsar-coder/backend-ifter/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx); err != nil {
		slog.Error("application_failed", "error", err)
		os.Exit(1)
	}
}
