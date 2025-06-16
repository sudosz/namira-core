package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/NaMiraNet/rayping/internal/core"
	"github.com/NaMiraNet/rayping/internal/crypto"
	"github.com/NaMiraNet/rayping/internal/logger"
	"github.com/go-redis/redis/v8"
	"github.com/google/go-github/v45/github"
	"go.uber.org/zap"
)

const FILENAME = "results.txt"

type Updater struct {
	githubClient  *github.Client
	redisClient   *redis.Client
	repoOwner     string
	repoName      string
	encryptionKey []byte
	logger        *zap.Logger
}

type ScanResult struct {
	JobID     string             `json:"job_id"`
	Results   []core.CheckResult `json:"results"`
	Timestamp time.Time          `json:"timestamp"`
}

type JSONResult struct {
	JobID     string             `json:"job_id"`
	Timestamp time.Time          `json:"timestamp"`
	Results   []JSONConfigResult `json:"results"`
}

type JSONConfigResult struct {
	Index     int    `json:"index"`
	Status    string `json:"status"`
	Delay     int64  `json:"delay_ms"`
	Error     string `json:"error,omitempty"`
	Protocol  string `json:"protocol"`
	RawConfig string `json:"raw_config"`
}

func NewUpdater(githubToken string, redisClient *redis.Client, repoOwner, repoName string, encryptionKey []byte) (*Updater, error) {
	ctx := context.Background()
	ts := github.BasicAuthTransport{
		Username: strings.TrimSpace(githubToken),
	}

	client := github.NewClient(ts.Client())

	// Verify GitHub credentials
	_, _, err := client.Users.Get(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate with GitHub: %w", err)
	}

	log, err := logger.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to get logger: %w", err)
	}

	return &Updater{
		githubClient:  client,
		redisClient:   redisClient,
		repoOwner:     repoOwner,
		repoName:      repoName,
		encryptionKey: encryptionKey,
		logger:        log,
	}, nil
}

func (u *Updater) ProcessScanResults(jobID string) error {
	ctx := context.Background()

	// Fetch results from Redis
	resultsKey := fmt.Sprintf("scan_results:%s", jobID)
	resultsData, err := u.redisClient.Get(ctx, resultsKey).Bytes()
	if err != nil {
		return fmt.Errorf("failed to fetch results from Redis: %w", err)
	}

	var scanResult ScanResult
	if err := json.Unmarshal(resultsData, &scanResult); err != nil {
		return fmt.Errorf("failed to unmarshal scan results: %w", err)
	}

	// Create JSON content
	jsonContent := formatResultsJSON(scanResult)

	// Encrypt JSON content
	encryptedJSON, err := crypto.Encrypt([]byte(jsonContent), u.encryptionKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt JSON results: %w", err)
	}

	// Create temporary file
	tmpDir := os.TempDir()
	jsonFile := filepath.Join(tmpDir, FILENAME)

	if err := os.WriteFile(jsonFile, encryptedJSON, 0644); err != nil {
		return fmt.Errorf("failed to write temporary JSON file: %w", err)
	}
	defer os.Remove(jsonFile)

	// Read JSON file
	jsonContentBytes, err := os.ReadFile(jsonFile)
	if err != nil {
		return fmt.Errorf("failed to read temporary JSON file: %w", err)
	}

	// Get the current file SHA if it exists
	var currentSHA *string
	file, _, _, err := u.githubClient.Repositories.GetContents(ctx, u.repoOwner, u.repoName, FILENAME, nil)
	if err == nil && file != nil {
		currentSHA = file.SHA
	}

	// Create or update the file on GitHub
	opts := &github.RepositoryContentFileOptions{
		Message: github.String(fmt.Sprintf("Update scan results - Job %s", jobID)),
		Content: []byte(base64.StdEncoding.EncodeToString(jsonContentBytes)),
		Branch:  github.String("main"),
	}

	if currentSHA != nil {
		opts.SHA = currentSHA
	}

	_, _, err = u.githubClient.Repositories.CreateFile(ctx, u.repoOwner, u.repoName, FILENAME, opts)
	if err != nil {
		return fmt.Errorf("failed to update on GitHub: %w", err)
	}

	u.logger.Info("Successfully updated results on GitHub",
		zap.String("job_id", jobID),
		zap.String("repo", fmt.Sprintf("%s/%s", u.repoOwner, u.repoName)))

	return nil
}

func formatResultsJSON(scanResult ScanResult) string {
	jsonResult := JSONResult{
		JobID:     scanResult.JobID,
		Timestamp: scanResult.Timestamp,
		Results:   make([]JSONConfigResult, len(scanResult.Results)),
	}

	for i, result := range scanResult.Results {
		jsonResult.Results[i] = JSONConfigResult{
			Index:     i + 1,
			Status:    string(result.Status),
			Delay:     result.RealDelay.Milliseconds(),
			Protocol:  result.Protocol,
			RawConfig: result.Raw,
		}
		if result.Error != "" {
			jsonResult.Results[i].Error = result.Error
		}
	}

	jsonData, err := json.MarshalIndent(jsonResult, "", "  ")
	if err != nil {
		return "{}"
	}

	return string(jsonData)
}
