package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NaMiraNet/rayping/internal/api"
	"github.com/NaMiraNet/rayping/internal/core"
	"github.com/NaMiraNet/rayping/internal/logger"
	"github.com/go-redis/redis/v8"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Start the API server",
	Long:  `Start the RayPing API server to handle VPN link checking requests.`,
	Run:   runAPIServer,
}

func runAPIServer(cmd *cobra.Command, args []string) {
	// Use global config, but allow CLI flags to override
	serverPort := port
	if serverPort == "" {
		serverPort = cfg.Server.Port
	}

	logger, err := logger.Init(cfg.App.LogLevel)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		logger.Fatal("Failed to connect to Redis", zap.Error(err))
	}
	logger.Info("Connected to Redis successfully", zap.String("addr", cfg.Redis.Addr))

	coreInstance := core.NewCore()

	// Define callback handler for completed jobs
	callbackHandler := func(result api.CallbackHandlerResult) {
		if result.Error != nil {
			logger.Error("Job failed", zap.String("job_id", result.JobID), zap.Error(result.Error))
		} else {
			logger.Info("Jobs Completed", zap.String("job_id", result.JobID), zap.Int("total", len(result.Results)))
		}
	}

	router := api.NewRouter(coreInstance, redisClient, callbackHandler, logger)

	serverAddr := fmt.Sprintf("%s:%s", cfg.Server.Host, serverPort)
	server := &http.Server{
		Addr:         serverAddr,
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	go func() {
		logger.Info("Server starting",
			zap.String("address", server.Addr),
			zap.Duration("read_timeout", cfg.Server.ReadTimeout),
			zap.Duration("write_timeout", cfg.Server.WriteTimeout),
		)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Server failed to start", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Server shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}
}
