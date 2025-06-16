package github

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/NaMiraNet/rayping/internal/core"
	"github.com/NaMiraNet/rayping/internal/crypto"
	"github.com/NaMiraNet/rayping/internal/logger"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

const (
	FILENAME     = "results.txt"
	CLONE_DEPTH  = 1
	FILE_PERMS   = 0644
	BOT_NAME     = "RayPing Bot"
	BOT_EMAIL    = "namiranet@proton.me"
	REMOTE_NAME  = "origin"
)

type Updater struct {
	auth          *ssh.PublicKeys
	redisClient   *redis.Client
	repoOwner     string
	repoName      string
	encryptionKey []byte
	logger        *zap.Logger
	workDir       string
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

func NewUpdater(sshKeyPath string, redisClient *redis.Client, repoOwner, repoName string, encryptionKey []byte) (*Updater, error) {
	if _, err := os.Stat(sshKeyPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("SSH key file not found: %s", sshKeyPath)
	}

	// Setup SSH authentication
	auth, err := ssh.NewPublicKeysFromFile("git", sshKeyPath, "")
	if err != nil {
		return nil, fmt.Errorf("failed to load SSH key: %w", err)
	}

	log, err := logger.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to get logger: %w", err)
	}

	return &Updater{
		auth:          auth,
		redisClient:   redisClient,
		repoOwner:     repoOwner,
		repoName:      repoName,
		encryptionKey: encryptionKey,
		logger:        log,
		workDir:       fmt.Sprintf("/tmp/rayping-updater-%s-%s", repoOwner, repoName),
	}, nil
}

// HealthCheck tests SSH connectivity to GitHub
func (u *Updater) HealthCheck() error {
	tempDir := u.workDir + "-healthcheck"
	defer os.RemoveAll(tempDir)

	_, err := git.PlainClone(tempDir, false, &git.CloneOptions{
		URL:   fmt.Sprintf("git@github.com:%s/%s.git", u.repoOwner, u.repoName),
		Auth:  u.auth,
		Depth: CLONE_DEPTH,
	})
	if err != nil {
		return fmt.Errorf("SSH connectivity test failed: %w", err)
	}

	u.logger.Info("SSH connectivity test passed")
	return nil
}

func (u *Updater) ProcessScanResults(jobID string) error {
	resultsData, err := u.fetchResults(jobID)
	if err != nil {
		return err
	}

	encryptedJSON, err := u.prepareContent(resultsData)
	if err != nil {
		return err
	}

	if err := u.updateFileViaGit(jobID, encryptedJSON); err != nil {
		return err
	}

	u.logger.Info("Successfully updated results on GitHub",
		zap.String("job_id", jobID),
		zap.String("repo", fmt.Sprintf("%s/%s", u.repoOwner, u.repoName)))

	return nil
}

func (u *Updater) fetchResults(jobID string) ([]byte, error) {
	resultsKey := fmt.Sprintf("scan_results:%s", jobID)
	resultsData, err := u.redisClient.Get(context.Background(), resultsKey).Bytes()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch results from Redis: %w", err)
	}
	return resultsData, nil
}

func (u *Updater) prepareContent(resultsData []byte) ([]byte, error) {
	var scanResult ScanResult
	if err := json.Unmarshal(resultsData, &scanResult); err != nil {
		return nil, fmt.Errorf("failed to unmarshal scan results: %w", err)
	}

	jsonContent := formatResultsJSON(scanResult)
	encryptedJSON, err := crypto.Encrypt([]byte(jsonContent), u.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt JSON results: %w", err)
	}

	return encryptedJSON, nil
}

func (u *Updater) updateFileViaGit(jobID string, content []byte) error {
	os.RemoveAll(u.workDir)
	defer os.RemoveAll(u.workDir)

	repo, err := git.PlainClone(u.workDir, false, &git.CloneOptions{
		URL:   fmt.Sprintf("git@github.com:%s/%s.git", u.repoOwner, u.repoName),
		Auth:  u.auth,
		Depth: CLONE_DEPTH,
	})
	if err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	encodedContent := base64.StdEncoding.EncodeToString(content)
	filePath := filepath.Join(u.workDir, FILENAME)
	if err := os.WriteFile(filePath, []byte(encodedContent), FILE_PERMS); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	if _, err := worktree.Add(FILENAME); err != nil {
		return fmt.Errorf("failed to add file: %w", err)
	}

	_, err = worktree.Commit(fmt.Sprintf("🤖 Update scan results - Job %s", jobID), &git.CommitOptions{
		Author: &object.Signature{
			Name:  BOT_NAME,
			Email: BOT_EMAIL,
			When:  time.Now(),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit: %w", err)
	}

	if err := repo.Push(&git.PushOptions{
		RemoteName: REMOTE_NAME,
		Auth:       u.auth,
	}); err != nil {
		return fmt.Errorf("failed to push: %w", err)
	}

	return nil
}

func formatResultsJSON(scanResult ScanResult) string {
	results := make([]JSONConfigResult, len(scanResult.Results))
	for i, result := range scanResult.Results {
		results[i] = JSONConfigResult{
			Index:     i + 1,
			Status:    string(result.Status),
			Delay:     result.RealDelay.Milliseconds(),
			Protocol:  result.Protocol,
			RawConfig: result.Raw,
			Error:     result.Error,
		}
	}

	jsonResult := JSONResult{
		JobID:     scanResult.JobID,
		Timestamp: scanResult.Timestamp,
		Results:   results,
	}

	jsonData, err := json.MarshalIndent(jsonResult, "", "  ")
	if err != nil {
		return "{}"
	}

	return string(jsonData)
}
