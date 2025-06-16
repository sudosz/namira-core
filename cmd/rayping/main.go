package main

import (
	"fmt"
	"log"
	"runtime"
	"strings"
	"time"

	"github.com/NaMiraNet/rayping/internal/config"
	"github.com/NaMiraNet/rayping/internal/github"
	"github.com/NaMiraNet/rayping/internal/logger"
	"github.com/go-redis/redis/v8"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	name    = "RayPing"
	build   = "Custom"
	version = "1.0.0"
)

// Version returns version
func Version() string {
	return version
}

// VersionStatement returns a list of strings representing the full version info.
func VersionStatement() string {
	return strings.Join([]string{
		name, " ", Version(), " ", build, " (", runtime.Version(), " ", runtime.GOOS, "/", runtime.GOARCH, ")",
	}, "")
}

var (
	port          string
	timeout       time.Duration
	maxConcurrent int

	cfg         *config.Config
	updater     *github.Updater
	redisClient *redis.Client
	appLogger   *zap.Logger
)

var rootCmd = &cobra.Command{
	Use:   name,
	Short: "RayPing VPN Link Checker Service",
	Long:  `A service to check and validate various VPN protocol links including Vmess, Vless, Shadowsocks, and Trojan.`,
}

func init() {
	fmt.Println(VersionStatement())

	// Load configuration from environment variables
	cfg = config.Load()

	// Initialize logger
	var err error
	appLogger, err = logger.Init(cfg.App.LogLevel)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	// Initialize Redis client
	redisClient = redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})

	// Initialize GitHub updater
	encryptionKey := []byte(cfg.App.EncryptionKey)
	updater, err = github.NewUpdater(
		cfg.Github.SSHKeyPath,
		redisClient,
		cfg.Github.Owner,
		cfg.Github.Repo,
		encryptionKey,
	)
	if err != nil {
		appLogger.Fatal("Failed to create updater:", zap.Error(err))
	}
	
	if err := updater.HealthCheck(); err != nil {
		appLogger.Fatal("GitHub SSH connectivity test failed:", zap.Error(err))
	}

	appLogger.Info("GitHub updater initialized successfully",
		zap.String("repo", fmt.Sprintf("%s/%s", cfg.Github.Owner, cfg.Github.Repo)),
		zap.String("ssh_key", cfg.Github.SSHKeyPath))

	rootCmd.PersistentFlags().StringVarP(&port, "port", "p", cfg.Server.Port, "Port to run the service on")
	rootCmd.PersistentFlags().DurationVarP(&timeout, "timeout", "t", cfg.App.Timeout, "Connection timeout")
	rootCmd.PersistentFlags().IntVarP(&maxConcurrent, "concurrent", "c", cfg.App.MaxConcurrent, "Maximum concurrent connections")

	// Add the API server subcommand
	rootCmd.AddCommand(apiCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
