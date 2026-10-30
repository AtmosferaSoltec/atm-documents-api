package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AtmosferaSoltec/atm-documents-api/internal/auth"
	"github.com/AtmosferaSoltec/atm-documents-api/internal/config"
	"github.com/AtmosferaSoltec/atm-documents-api/internal/platform/httpserver"
	"github.com/AtmosferaSoltec/atm-documents-api/internal/renderer"
	"github.com/AtmosferaSoltec/atm-documents-api/internal/salesnote"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		logger.Error("unable to load .env")
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, logger, os.Args[1:]); err != nil {
		logger.Error("documents service stopped", "error", err.Error())
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger, args []string) error {
	if len(args) > 1 || (len(args) == 1 && args[0] != "serve") {
		return errors.New("usage: api [serve]")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	verifier, err := auth.New(cfg.AuthPublicKeyBase64, cfg.AuthIssuer, cfg.AllowedSystems)
	if err != nil {
		return err
	}

	service := salesnote.NewService(renderer.New(), cfg.MaxConcurrentRenders)
	handler := salesnote.NewHandler(service, cfg.RenderTimeout, logger)

	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	if err := engine.SetTrustedProxies(nil); err != nil {
		return err
	}
	// A custom recovery avoids Gin logging authorization headers on panic.
	engine.Use(func(c *gin.Context) {
		defer func() {
			if recover() != nil {
				logger.Error("request panic")
				httpserver.Error(c, 500, "internal_error", "Unable to complete the request.")
			}
		}()
		c.Next()
	}, httpserver.RequestID())

	type statusResponse struct {
		Status string `json:"status"`
	}
	engine.GET("/health", func(c *gin.Context) { c.JSON(200, statusResponse{Status: "ok"}) })
	// The service holds no database or remote dependency: when it runs, it is ready.
	engine.GET("/ready", func(c *gin.Context) { c.JSON(200, statusResponse{Status: "ready"}) })
	salesnote.RegisterRoutes(engine.Group("/v1", verifier.Middleware()), handler)
	engine.NoRoute(func(c *gin.Context) { httpserver.Error(c, 404, "not_found", "Route not found.") })

	server := &http.Server{
		Addr:              ":" + cfg.HTTPPort,
		Handler:           engine,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      cfg.RenderTimeout + 15*time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	result := make(chan error, 1)
	go func() { result <- server.ListenAndServe() }()
	logger.Info("documents service started", "port", cfg.HTTPPort, "environment", cfg.Environment, "systems", cfg.AllowedSystems)

	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("listen: %w", err)
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return errors.New("graceful shutdown timed out")
	}
	return nil
}
