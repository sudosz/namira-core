package main

import (
	"context"
	"fmt"

	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/NaMiraNet/namira-core/internal/api"
	"github.com/NaMiraNet/namira-core/internal/core"
	"github.com/NaMiraNet/namira-core/internal/logger"
	"github.com/NaMiraNet/namira-core/internal/notify"
	workerpool "github.com/NaMiraNet/namira-core/internal/worker"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

var apiCmd = &cobra.Command{
	Use:   "api",
	Short: "Start the API server",
	Long:  `Start the namira-core API server to handle VPN link checking requests.`,
	Run:   runAPIServer,
}

func runAPIServer(cmd *cobra.Command, args []string) {
	// Use global config, but allow CLI flags to override
	serverPort := port
	if serverPort == "" {
		serverPort = cfg.Server.Port
	}

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

	telegramTransport := &http.Transport{}
	if proxyURL := cfg.Telegram.ProxyURL; proxyURL != "" {
		proxy, err := url.Parse(proxyURL)
		if err != nil {
			logger.Fatal("Failed to parse proxy URL", zap.Error(err))
		}
		telegramTransport.Proxy = http.ProxyURL(proxy)
	}

	telegram := notify.NewTelegram(
		cfg.Telegram.BotToken,
		cfg.Telegram.Channel,
		cfg.Telegram.Template,
		cfg.Telegram.QRConfig,
		&http.Client{
			Timeout:   10 * time.Second,
			Transport: telegramTransport,
		},
	)

	tgLimiter := rate.NewLimiter(rate.Every(cfg.Telegram.SendingInterval), 1)

	telegramConfigResultHandler := func(result core.CheckResult) {
		// ignore if error or delay too long
		if result.Error != "" || result.RealDelay > 3*time.Second {
			return
		}
		go func() {
			if err := tgLimiter.Wait(context.Background()); err != nil {
				logger.Error("Rate limit error", zap.Error(err))
				return
			}

			if err := telegram.SendWithQRCode(result); err != nil {
				logger.Error("Failed to send Telegram notification", zap.Error(err))
			}
		}()
	}
	// worker instace
	worker := workerpool.NewWorkerPool(workerpool.WorkerPoolConfig{
		WorkerCount:   cfg.Worker.Count,
		TaskQueueSize: cfg.Worker.QueueSize,
	})

	router := api.NewRouter(
		coreInstance,
		redisClient,
		callbackHandler,
		telegramConfigResultHandler,
		appLogger,
		updater,
		worker)

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
