package main

import (
	"fmt"
	"log"
	"runtime"
	"strings"
	"time"

	"github.com/NaMiraNet/rayping/internal/config"
	"github.com/spf13/cobra"
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
	cfg           *config.Config
	port          string
	timeout       time.Duration
	maxConcurrent int
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
