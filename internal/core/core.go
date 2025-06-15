package core

import (
	"sort"
	"sync"
	"time"

	"github.com/NaMiraNet/rayping/internal/core/parser"
	"github.com/NaMiraNet/rayping/internal/core/tester"
	"slices"
)

type TestResultStatusType string

const (
	TestResultStatusSuccess     TestResultStatusType = "success"
	TestResultStatusUnavailable TestResultStatusType = "unavailable"
	TestResultStatusError       TestResultStatusType = "error"
)

type Config interface {
	MarshalJSON() ([]byte, error)
}

type TestResult struct {
	Status    TestResultStatusType
	RealDelay time.Duration
	Error     error
}

type Core struct {
	tester             tester.ConfigTester
	parser             *parser.Parser
	maxConcurrentTests int
}

type CoreOpts struct {
	TestTimeout       time.Duration
	TestServer        string
	TestPort          uint32
	TestMaxConcurrent int
}

func NewCore(opts ...CoreOpts) *Core {
	if len(opts) == 0 {
		opts = append(opts, CoreOpts{
			TestTimeout:       10 * time.Second,
			TestServer:        "1.1.1.1",
			TestPort:          80,
			TestMaxConcurrent: 10,
		})
	}
	return &Core{
		tester:             tester.NewV2RayConfigTester(opts[0].TestTimeout, opts[0].TestServer, opts[0].TestPort),
		parser:             parser.NewParser(),
		maxConcurrentTests: opts[0].TestMaxConcurrent,
	}
}

func (c *Core) TestConfigs(configs []string) <-chan TestResult {
	results := make(chan TestResult)
	var wg sync.WaitGroup
	sem := make(chan struct{}, c.maxConcurrentTests)
	go func() {
		for i, config := range configs {
			wg.Add(1)
			sem <- struct{}{} // Acquire semaphore
			go func(index int, cfg string) {
				defer wg.Done()
				defer func() { <-sem }() // Release semaphore

				result := TestResult{
					Status: TestResultStatusSuccess,
				}

				parsed, err := c.parser.Parse(cfg)
				if err != nil {
					result.Status = TestResultStatusError
					result.Error = err
					results <- result
					return
				}

				delay, err := c.tester.TestConfig(parsed)
				if err != nil {
					result.Status = TestResultStatusError
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

func (c *Core) TestConfigsList(configs []string) []TestResult {
	results := make([]TestResult, 0)
	for result := range c.TestConfigs(configs) {
		results = append(results, result)
	}
	return results
}

func (c *Core) SortTestResultList(results []TestResult) []TestResult {
	results = slices.Clone(results)
	sort.Slice(results, func(i, j int) bool {
		if results[i].Status != results[j].Status {
			return results[i].Status == TestResultStatusSuccess
		}
		return results[i].RealDelay < results[j].RealDelay
	})
	return results
}
