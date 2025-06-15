package core

import (
	"sort"
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
	RealDelay time.Duration
	Error     error
}

type Core struct {
	checker             checker.ConfigChecker
	parser              *parser.Parser
	maxConcurrentChecks int
}

type CoreOpts struct {
	CheckTimeout       time.Duration
	CheckServer        string
	CheckPort          uint32
	CheckMaxConcurrent int
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
	return &Core{
		checker:             checker.NewV2RayConfigChecker(opts[0].CheckTimeout, opts[0].CheckServer, opts[0].CheckPort),
		parser:              parser.NewParser(),
		maxConcurrentChecks: opts[0].CheckMaxConcurrent,
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
					Status: CheckResultStatusSuccess,
				}

				parsed, err := c.parser.Parse(cfg)
				if err != nil {
					result.Status = CheckResultStatusError
					result.Error = err
					results <- result
					return
				}

				delay, err := c.checker.CheckConfig(parsed)
				if err != nil {
					result.Status = CheckResultStatusError
					result.Error = err
				} else {
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
