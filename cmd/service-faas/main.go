package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"service-faas/internal/adapters/docker"
	"service-faas/internal/adapters/gorm"
	"service-faas/internal/config"
	"service-faas/internal/core/functions"
	api "service-faas/internal/delivery/http"

	_ "service-faas/docs"

	"github.com/rs/zerolog"
)

// @title           FaaS Manager API
// @version         1.0
// @description     API for managing and executing functions as a service.
// @host            localhost:8080
// @BasePath        /
func main() {
	log := zerolog.New(os.Stdout).With().Timestamp().
		Str("svc", "service-faas").Logger()

	cfg := config.MustLoad()
	log.Info().
		Str("deployment_env", string(cfg.DeploymentEnv)).
		Msg("bootstrapping service")

	db, err := gorm.New(cfg.DatabaseDSN, log)
	if err != nil {
		log.Fatal().Err(err).Msg("gorm connect")
	}

	// Conditionally initialize clients based on environment
	var dcli *docker.Client
	if cfg.DeploymentEnv == config.EnvDocker {
		var err error
		dcli, err = docker.New(cfg, log)
		if err != nil {
			log.Fatal().Err(err).Msg("docker client init")
		}
	} else if cfg.DeploymentEnv == config.EnvKubernetes {
		// Placeholder for when you add the Kubernetes client
		log.Fatal().Msg("kubernetes client not yet implemented")
	}

	mgr := functions.NewManager(db, dcli, cfg, log)

	// On startup, restart any functions that were running before.
	if err := mgr.RestartRunningFunctions(context.Background()); err != nil {
		log.Error().Err(err).Msg("error during function restart")
	}

	handler := api.NewHandler(mgr, log)
	srv := &http.Server{Addr: cfg.ListenAddr, Handler: handler}

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(
		context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Info().Str("listen", cfg.ListenAddr).Msg("HTTP server starting")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("http server failed")
		}
	}()

	<-ctx.Done()

	log.Info().Msg("shutting down server...")
	_ = srv.Shutdown(context.Background())

	// Cleanup running containers
	if err := mgr.CleanupAllFunctions(context.Background()); err != nil {
		log.Error().Err(err).Msg("error during function cleanup")
	}

	log.Info().Msg("shutdown complete")
}
