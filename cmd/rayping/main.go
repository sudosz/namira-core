package main

import (
	"log"
	"time"

	"github.com/NaMiraNet/rayping/internal/service"
	"github.com/spf13/cobra"
)

var (
	port          string
	timeout       time.Duration
	maxConcurrent int
)

var rootCmd = &cobra.Command{
	Use:   "rayping",
	Short: "RayPing VPN Link Tester Service",
	Long:  `A service to test and validate various VPN protocol links including Vmess, Vless, Shadowsocks, and Trojan.`,
	Run: func(cmd *cobra.Command, args []string) {
		srv := service.NewRayPingService(timeout, maxConcurrent)
		if err := srv.StartServer(port); err != nil {
			log.Fatal("Failed to start server:", err)
		}
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&port, "port", "p", "8080", "Port to run the service on")
	rootCmd.PersistentFlags().DurationVarP(&timeout, "timeout", "t", 10, "Connection timeout in seconds")
	rootCmd.PersistentFlags().IntVarP(&maxConcurrent, "concurrent", "c", 50, "Maximum concurrent connections")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
