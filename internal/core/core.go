package core

import (
	"sort"
	"strings"
	"sync"
	"time"

	"slices"

	"github.com/NaMiraNet/rayping/internal/core/checker"
	"github.com/NaMiraNet/rayping/internal/core/parser"
)

type CheckResultStatusType string

const (
	CheckResultStatusSuccess     CheckResultStatusType = "success"
	CheckResultStatusUnavailable CheckResultStatusType = "unavailable"
	CheckResultStatusError       CheckResultStatusType = "error"
)

type Config interface {
	MarshalJSON() ([]byte, error)
}

type CheckResult struct {
	Status    CheckResultStatusType
	Protocol  string
	Raw       string
	RealDelay time.Duration
	Error     string
}

type Core struct {
	checker             checker.ConfigChecker
	parser              *parser.Parser
	remarkTemplate      RemarkTemplate
	maxConcurrentChecks int
}

type CoreOpts struct {
	CheckTimeout       time.Duration
	CheckServer        string
	CheckPort          uint32
	CheckMaxConcurrent int
	RemarkTemplate     *RemarkTemplate
}

func NewCore(opts ...CoreOpts) *Core {
	if len(opts) == 0 {
		opts = append(opts, CoreOpts{
			CheckTimeout:       10 * time.Second,
			CheckServer:        "1.1.1.1",
			CheckPort:          80,
			CheckMaxConcurrent: 10,
		})
	}

	remarkTemplate := DefaultRemarkTemplate()
	if opts[0].RemarkTemplate != nil {
		remarkTemplate = *opts[0].RemarkTemplate
	}

	return &Core{
		checker:             checker.NewV2RayConfigChecker(opts[0].CheckTimeout, opts[0].CheckServer, opts[0].CheckPort),
		parser:              parser.NewParser(),
		maxConcurrentChecks: opts[0].CheckMaxConcurrent,
		remarkTemplate:      remarkTemplate,
	}
}

func (c *Core) CheckConfigs(configs []string) <-chan CheckResult {
	results := make(chan CheckResult)
	var wg sync.WaitGroup
	sem := make(chan struct{}, c.maxConcurrentChecks)
	go func() {
		for i, config := range configs {
			wg.Add(1)
			sem <- struct{}{} // Acquire semaphore
			go func(index int, cfg string) {
				defer wg.Done()
				defer func() { <-sem }() // Release semaphore

				result := CheckResult{
					Status:   CheckResultStatusSuccess,
					Protocol: strings.ToLower(strings.SplitN(cfg, "://", 2)[0]),
					Raw:      cfg,
				}

				parsed, err := c.parser.Parse(cfg)

				if err != nil {
					result.Status = CheckResultStatusError
					result.Error = err.Error()
					results <- result
					return
				}

				delay, err := c.checker.CheckConfig(parsed)
				if err != nil {
					result.Status = CheckResultStatusError
					result.Error = err.Error()
				} else {
					processedConfig := c.ReplaceConfigRemark(cfg)
					result.Raw = processedConfig
					result.Protocol = strings.ToLower(strings.SplitN(cfg, "://", 2)[0])
					result.RealDelay = delay
				}
				results <- result
			}(i, config)
		}
		wg.Wait()
		close(results)
	}()

	return results
}

func (c *Core) CheckConfigsList(configs []string) []CheckResult {
	results := make([]CheckResult, 0)
	for result := range c.CheckConfigs(configs) {
		results = append(results, result)
	}
	return results
}

func (c *Core) SortCheckResultList(results []CheckResult) []CheckResult {
	results = slices.Clone(results)
	sort.Slice(results, func(i, j int) bool {
		if results[i].Status != results[j].Status {
			return results[i].Status == CheckResultStatusSuccess
		}
		return results[i].RealDelay < results[j].RealDelay
	})
	return results
}
