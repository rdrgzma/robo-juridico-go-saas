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

	"github.com/rdrgzma/robo-juridico-go-saas/internal/config"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/storage"
	"github.com/rdrgzma/robo-juridico-go-saas/internal/transport"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("configuration", "error", err)
		os.Exit(1)
	}
	connectCtx, cancelConnect := context.WithTimeout(context.Background(), 10*time.Second)
	pool, err := storage.Connect(connectCtx, cfg.DatabaseURL)
	cancelConnect()
	if err != nil {
		logger.Error("database unavailable")
		os.Exit(1)
	}
	defer pool.Close()
	ready := func(ctx context.Context) error { return storage.Ready(ctx, pool) }
	server := &http.Server{Addr: cfg.Address, Handler: transport.New(cfg.APIKeyHash, logger, ready), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-ctx.Done()
		deadline, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(deadline); err != nil {
			logger.Error("shutdown", "error", err)
			_ = server.Close()
		}
	}()
	logger.Info("listening", "address", cfg.Address, "sprint", 1)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server", "error", err)
		os.Exit(1)
	}
	<-stopped
}
