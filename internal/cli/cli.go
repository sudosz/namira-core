package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/NamiraNet/namira-core/internal/core"
)

type CLI struct {
	core *core.Core
}

func NewCLI(coreInstance *core.Core) *CLI {
	return &CLI{
		core: coreInstance,
	}
}

type ConfigReader struct{}

func NewConfigReader() *ConfigReader {
	return &ConfigReader{}
}

func (cr *ConfigReader) File(filename string) ([]string, error) {
	if filename == "" {
		return []string{}, nil
	}

	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var configs []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && !strings.HasPrefix(line, "#") {
			configs = append(configs, line)
		}
	}
	return configs, scanner.Err()
}

func (cr *ConfigReader) Stdin(configs []string) ([]string, error) {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return configs, nil
	}

	if (stat.Mode() & os.ModeCharDevice) == 0 {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" && !strings.HasPrefix(line, "#") {
				configs = append(configs, line)
			}
		}
		return configs, scanner.Err()
	}

	return configs, nil
}

func (cr *ConfigReader) ReadConfigs(filename string, configArgs []string) ([]string, string, error) {
	// check and read from stdin
	stdinConfigs, err := cr.Stdin([]string{})
	if err != nil {
		return nil, "", fmt.Errorf("error reading from stdin: %v", err)
	}
	if len(stdinConfigs) > 0 {
		return stdinConfigs, "stdin", nil
	}

	// read file
	if filename != "" {
		fileConfigs, err := cr.File(filename)
		if err != nil {
			return nil, "", fmt.Errorf("error reading from file %s: %v", filename, err)
		}
		if len(fileConfigs) > 0 {
			return fileConfigs, "file", nil
		}
	}

	// read from config arg
	var configsFromArgs []string
	for _, config := range configArgs {
		config = strings.TrimSpace(config)
		if config != "" && !strings.HasPrefix(config, "#") {
			configsFromArgs = append(configsFromArgs, config)
		}
	}
	if len(configsFromArgs) > 0 {
		return configsFromArgs, "arguments", nil
	}

	return []string{}, "none", nil
}

type ConfigProcessor struct{}

func NewConfigProcessor() *ConfigProcessor {
	return &ConfigProcessor{}
}

func (cp *ConfigProcessor) RemoveDuplicates(configs []string) []string {
	seen := make(map[string]bool)
	var unique []string
	for _, config := range configs {
		config = strings.TrimSpace(config)
		if config != "" && !seen[config] {
			seen[config] = true
			unique = append(unique, config)
		}
	}
	return unique
}

type CheckOptions struct {
	ShowProgress bool
}

type Checker struct {
	core *core.Core
}

func NewChecker(coreInstance *core.Core) *Checker {
	return &Checker{
		core: coreInstance,
	}
}

// PerformChecks executes checks on configurations with optional progress display
func (c *Checker) PerformChecks(configs []string, options CheckOptions) []core.CheckResult {
	var results []core.CheckResult
	processed := 0
	total := len(configs)

	for result := range c.core.CheckConfigs(configs) {
		results = append(results, result)
		processed++

		if options.ShowProgress {
			status := core.CheckResultStatusSuccess
			if result.Error != "" {
				status = core.CheckResultStatusError
			}
			fmt.Printf("\r[%s] Progress: %d/%d (%.1f%%) - Last: %s",
				status, processed, total,
				float64(processed)/float64(total)*100,
				truncateString(result.Raw, 50))
		}
	}

	if options.ShowProgress {
		fmt.Println()
	}

	return results
}

type OutputOptions struct {
	Format   string
	Filename string
}

type OutputManager struct{}

func NewOutputManager() *OutputManager {
	return &OutputManager{}
}

func (om *OutputManager) Output(results []core.CheckResult, options OutputOptions) error {
	var output string
	var err error

	switch options.Format {
	case "json":
		output, err = om.JSON(results)
	case "csv":
		output, err = om.CSV(results)
	case "table":
		output = om.Table(results)
	default:
		return fmt.Errorf("unsupported output format: %s", options.Format)
	}

	if err != nil {
		return err
	}

	if options.Filename != "" {
		return os.WriteFile(options.Filename, []byte(output), 0644)
	}

	fmt.Print(output)
	return nil
}

func (om *OutputManager) JSON(results []core.CheckResult) (string, error) {
	data, err := json.MarshalIndent(results, "", "  ")
	return string(data), err
}

func (om *OutputManager) CSV(results []core.CheckResult) (string, error) {
	var lines []string
	lines = append(lines, "Status,Server,Protocol,Country,Delay(ms),Error,Config")

	for _, result := range results {
		delay := ""
		if result.RealDelay > 0 {
			delay = fmt.Sprintf("%.0f", result.RealDelay.Seconds()*1000)
		}

		line := fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s",
			result.Status,
			result.Server,
			result.Protocol,
			result.CountryCode,
			delay,
			result.Error,
			escapeCSV(result.Raw))
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n"), nil
}

func (om *OutputManager) Table(results []core.CheckResult) string {
	var lines []string

	// Header
	lines = append(lines, fmt.Sprintf("%-8s %-20s %-10s %-8s %-10s %-50s",
		"STATUS", "SERVER", "PROTOCOL", "COUNTRY", "DELAY(ms)", "ERROR"))
	lines = append(lines, strings.Repeat("-", 120))

	// Results
	for _, result := range results {
		delay := "N/A"
		if result.RealDelay > 0 {
			delay = fmt.Sprintf("%.0f", result.RealDelay.Seconds()*1000)
		}

		errorMsg := truncateString(result.Error, 50)

		line := fmt.Sprintf("%-8s %-20s %-10s %-8s %-10s %-50s",
			result.Status,
			truncateString(result.Server, 20),
			result.Protocol,
			result.CountryCode,
			delay,
			errorMsg)
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n") + "\n"
}

type SummaryPrinter struct{}

func NewSummaryPrinter() *SummaryPrinter {
	return &SummaryPrinter{}
}

// PrintSummary prints a summary of the check results
func (sp *SummaryPrinter) PrintSummary(results []core.CheckResult) {
	total := len(results)
	successful := 0
	failed := 0
	var totalDelay time.Duration

	for _, result := range results {
		if result.Error == "" {
			successful++
			totalDelay += result.RealDelay
		} else {
			failed++
		}
	}

	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("SUMMARY")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("Total configurations: %d\n", total)
	fmt.Printf("Successful: %d (%.1f%%)\n", successful, float64(successful)/float64(total)*100)
	fmt.Printf("Failed: %d (%.1f%%)\n", failed, float64(failed)/float64(total)*100)

	if successful > 0 {
		avgDelay := totalDelay / time.Duration(successful)
		fmt.Printf("Average delay: %.0f ms\n", avgDelay.Seconds()*1000)
	}
}

// Utility functions
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func escapeCSV(s string) string {
	if strings.Contains(s, ",") || strings.Contains(s, "\"") || strings.Contains(s, "\n") {
		s = strings.ReplaceAll(s, "\"", "\"\"")
		return "\"" + s + "\""
	}
	return s
}
