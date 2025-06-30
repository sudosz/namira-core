package main

import (
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/NamiraNet/namira-core/internal/cli"
	"github.com/NamiraNet/namira-core/internal/core"
	"github.com/NamiraNet/namira-core/internal/logger"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	configFile   string
	configsInput []string
	outputFormat string
	outputFile   string
	showProgress bool
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "namira-core cli VPN Link Scanner",
	Long:  `A cli scanner to check and validate various VPN protocol links including Vmess, Vless, Shadowsocks, and Trojan`,
	Run:   runCli,
}

func init() {
	checkCmd.Flags().StringVarP(&configFile, "file", "i", "", "File containing VPN configurations (one per line)")
	checkCmd.Flags().StringSliceVarP(&configsInput, "config", "c", []string{}, "VPN configuration strings (can be used multiple times)")
	checkCmd.Flags().StringVarP(&outputFormat, "format", "f", "table", "Output format: table, json, csv")
	checkCmd.Flags().StringVarP(&outputFile, "output", "o", "", "Output file (default: stdout)")
	checkCmd.Flags().BoolVar(&showProgress, "progress", true, "Show progress during checking")
	checkCmd.Flags().IntVarP(&maxConcurrent, "concurrent", "j", 10, "Maximum concurrent checks")
	checkCmd.Flags().DurationVarP(&timeout, "timeout", "t", 10*time.Second, "Timeout for each check")
}

func runCli(cmd *cobra.Command, args []string) {
	logger, err := logger.InitForCLI(cfg.App.LogLevel)
	if err != nil {
		fmt.Println("Failed to initialize logger:", err)
		return
	}
	defer func() {
		if syncErr := logger.Sync(); syncErr != nil {
			fmt.Printf("Failed to sync logger: %v\n", syncErr)
		}
	}()

	var configs []string

	checkServer, checkP, err := net.SplitHostPort(cfg.App.CheckHost)
	if err != nil {
		logger.Fatal("Failed to parse check server", zap.Error(err))
	}

	checkPort, _ := strconv.Atoi(checkP)

	coreInstance := core.NewCore(core.CoreOpts{
		CheckTimeout:       cfg.App.Timeout,
		CheckServer:        checkServer,
		CheckPort:          uint32(checkPort),
		CheckMaxConcurrent: cfg.App.MaxConcurrent,
		RemarkTemplate: &core.RemarkTemplate{
			OrgName:      "@NamiraNet",
			Separator:    " | ",
			ShowCountry:  true,
			ShowHost:     true,
			ShowProtocol: true,
		},
	})

	// register cli
	cli.NewCLI(coreInstance)

	reader := cli.NewConfigReader()
	processor := cli.NewConfigProcessor()
	checker := cli.NewChecker(coreInstance)
	outputManager := cli.NewOutputManager()
	summaryPrinter := cli.NewSummaryPrinter()

	// read config
	configs, source, err := reader.ReadConfigs(configFile, configsInput)
	if err != nil {
		logger.Error("error reading configs: %v\n", zap.Error(err))
		return
	}
	// process configs
	uniqueConfigs := processor.RemoveDuplicates(configs)

	logger.Info("configuration reading completed",
		zap.String("source", source),
		zap.Int("total", len(configs)),
		zap.Int("unique", len(uniqueConfigs)))

	// check configs with core
	checkOptions := cli.CheckOptions{ShowProgress: showProgress}
	results := checker.PerformChecks(uniqueConfigs, checkOptions)

	// output results
	outputOptions := cli.OutputOptions{
		Format:   outputFormat,
		Filename: outputFile,
	}
	if err := outputManager.Output(results, outputOptions); err != nil {
		logger.Error("failed to output results", zap.Error(err))
		return
	}

	summaryPrinter.PrintSummary(results)

}
