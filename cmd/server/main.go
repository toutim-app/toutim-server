package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/toutim-app/toutim-server/internal/api"
	"github.com/toutim-app/toutim-server/internal/auth"
	"github.com/toutim-app/toutim-server/internal/config"
	"github.com/toutim-app/toutim-server/internal/database"
	"github.com/toutim-app/toutim-server/internal/email"
	"github.com/toutim-app/toutim-server/internal/export"
	"github.com/toutim-app/toutim-server/internal/files"
	"github.com/toutim-app/toutim-server/internal/sync"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "invalid config: %v\n", err)
		os.Exit(1)
	}

	// Setup structured logger
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Graceful shutdown context
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Initialize database manager
	dbMgr, err := database.NewDatabaseManager(ctx, cfg, logger)
	if err != nil {
		logger.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer dbMgr.Close()

	// Initialize email sender
	emailSender := email.NewSender(cfg.SMTP, logger)
	if emailSender.Enabled() {
		logger.Info("email service enabled", "smtp_host", cfg.SMTP.Host)
	} else {
		logger.Info("email service disabled (SMTP not configured)")
	}

	// Initialize auth
	jwtMgr := auth.NewJWTManager(cfg.Auth)
	authHandler := auth.NewHandler(dbMgr, jwtMgr, emailSender, cfg.SMTP, logger)

	// Initialize sync
	syncEngine := sync.NewEngine(dbMgr, logger, cfg.Sync.MaxChangesPerPush)
	syncHandler := sync.NewHandler(syncEngine, logger)

	// Initialize files
	maxFileSize := int64(10 * 1024 * 1024) // 10MB default
	fileHandler := files.NewHandler(cfg.Data.Dir, maxFileSize, logger)

	// Initialize export
	exportHandler := export.NewHandler(dbMgr, logger)

	// Build router
	router := api.NewRouter(logger, dbMgr, authHandler, jwtMgr, syncHandler, fileHandler, exportHandler)

	// Create HTTP server
	addr := net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port))
	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		<-ctx.Done()
		logger.Info("shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown error", "error", err)
		}
	}()

	logger.Info("server starting",
		"addr", addr,
		"data_dir", cfg.Data.Dir,
	)

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}

	logger.Info("server stopped")
}
