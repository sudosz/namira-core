package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/NaMiraNet/namira-core/internal/core"
	"github.com/NaMiraNet/namira-core/internal/github"
	workerpool "github.com/NaMiraNet/namira-core/internal/worker"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

const REDIS_SET_DURATION = 30 * time.Minute

type CallbackHandlerResult struct {
	JobID   string
	Results []core.CheckResult
	Error   error
}

type CallbackHandler func(CallbackHandlerResult)
type ConfigSuccessHandler func(core.CheckResult)

type Handler struct {
	core          *core.Core
	workerPool    *workerpool.WorkerPool
	redis         *redis.Client
	jobs          sync.Map
	logger        *zap.Logger
	updater       *github.Updater
	jobsOnSuccess ConfigSuccessHandler
}

func NewHandler(c *core.Core, redisClient *redis.Client, callbackHandler CallbackHandler, configSuccessHandler ConfigSuccessHandler, logger *zap.Logger, updater *github.Updater, worker *workerpool.WorkerPool) *Handler {
	handler := &Handler{
		core:          c,
		workerPool:    worker,
		redis:         redisClient,
		logger:        logger,
		updater:       updater,
		jobsOnSuccess: configSuccessHandler,
	}

	worker.SetResultHandler(handler.handleTaskResult(callbackHandler))
	if err := worker.Start(); err != nil {
		panic("Failed to start worker pool: " + err.Error())
	}
	return handler
}

func (h *Handler) handleScan(w http.ResponseWriter, r *http.Request) {
	var configs []string

	// Check content type
	contentType := r.Header.Get("Content-Type")
	if strings.Contains(contentType, "multipart/form-data") {
		file, _, err := r.FormFile("file")
		if err != nil {
			h.logger.Error("Failed to read file", zap.Error(err))
			writeError(w, "Failed to read file", http.StatusBadRequest)
			return
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				configs = append(configs, line)
			}
		}
		if err := scanner.Err(); err != nil {
			h.logger.Error("Failed to read file content", zap.Error(err))
			writeError(w, "Failed to read file content", http.StatusBadRequest)
			return
		}
	} else {
		// Handle JSON request
		var req ScanRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			h.logger.Error("Invalid Json", zap.Error(err))
			writeError(w, "Invalid JSON", http.StatusBadRequest)
			return
		}
		configs = req.Configs
	}

	if len(configs) == 0 {
		h.logger.Error("No configs provided")
		writeError(w, "No configs provided", http.StatusBadRequest)
		return
	}

	uniqueConfigs, err := h.filterDuplicates(configs)
	if err != nil {
		h.logger.Error("Failed to filter duplicates", zap.Error(err))
		writeError(w, "Failed to filter duplicates", http.StatusInternalServerError)
		return
	}

	if len(uniqueConfigs) == 0 {
		h.logger.Error("All configs are duplicates")
		writeError(w, "All configs are duplicates", http.StatusBadRequest)
		return
	}

	job := NewJob(uniqueConfigs)
	h.jobs.Store(job.ID, job)
	job.Start()

	if err := h.workerPool.Submit(workerpool.Task{
		ID:      job.ID,
		Data:    TaskData{JobID: job.ID, Configs: uniqueConfigs},
		Execute: h.executeCheckTask,
	}); err != nil {
		h.logger.Error("Failed to submit task", zap.Error(err))
		job.Fail(err)
		writeError(w, "Failed to submit task", http.StatusInternalServerError)
		return
	}

	writeJSON(w, ScanResponse{JobID: job.ID})
}

func (h *Handler) handleJobStatus(w http.ResponseWriter, r *http.Request) {
	if value, exists := h.jobs.Load(mux.Vars(r)["id"]); exists {
		writeJSON(w, value.(*Job))
		return
	}
	h.logger.Error("Job not found")
	writeError(w, "Job not found", http.StatusNotFound)
}

func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	stats := h.workerPool.GetStats()
	writeJSON(w, HealthResponse{
		Status:  "ok",
		Version: "1.0.0",
		WorkerPool: WorkerPoolStatus{
			WorkerCount:    stats.WorkerCount,
			TotalTasks:     stats.TotalTasks,
			CompletedTasks: stats.CompletedTasks,
			FailedTasks:    stats.FailedTasks,
			QueueLength:    stats.QueueLength,
			IsRunning:      stats.IsRunning,
			Uptime:         stats.Uptime.String(),
		},
	})
}

func (h *Handler) executeCheckTask(ctx context.Context, data interface{}) (interface{}, error) {
	taskData := data.(TaskData)
	value, exists := h.jobs.Load(taskData.JobID)
	if !exists {
		h.logger.Error("Job not found", zap.String("job_id", taskData.JobID))
		return nil, nil
	}

	job := value.(*Job)
	results := make([]core.CheckResult, 0, len(taskData.Configs))

	i := 0
	for result := range h.core.CheckConfigs(taskData.Configs) {
		results = append(results, result)
		checkResult := CheckResult{
			Index:  i,
			Status: string(result.Status),
			Delay:  result.RealDelay.Milliseconds(),
		}

		if result.Error != "" {
			h.logger.Error("error in check", zap.String("error", result.Error),
				zap.String("status", string(result.Status)),
				zap.String("server", result.Server),
				zap.String("protocol", result.Protocol))
			job.Done()
		} else {
			h.logger.Info("link response", zap.String("config", result.Raw))
			job.AddResult(HashConfig(result.Raw), checkResult)
			h.jobsOnSuccess(result)
		}

		if job.DoneCount >= job.TotalCount {
			job.Complete()
			h.logger.Info("Job completed", zap.String("job_id", taskData.JobID))
		}
		i++
	}

	return CallbackHandlerResult{
		JobID:   taskData.JobID,
		Results: results,
	}, nil
}

func (h *Handler) handleTaskResult(callback CallbackHandler) func(workerpool.Result) {
	return func(result workerpool.Result) {
		if result.Error == nil {
			callbackResult := result.Result.(CallbackHandlerResult)

			// Store results in Redis
			scanResult := github.ScanResult{
				JobID:     callbackResult.JobID,
				Results:   callbackResult.Results,
				Timestamp: time.Now(),
			}

			resultsData, err := json.Marshal(scanResult)
			if err != nil {
				h.logger.Error("Failed to marshal scan results", zap.Error(err))
				return
			}

			ctx := context.Background()
			resultsKey := fmt.Sprintf("scan_results:%s", callbackResult.JobID)
			if err := h.redis.Set(ctx, resultsKey, resultsData, REDIS_SET_DURATION).Err(); err != nil {
				h.logger.Error("Failed to store results in Redis", zap.Error(err))
				return
			}

			// Trigger GitHub update
			if err := h.updater.ProcessScanResults(callbackResult.JobID); err != nil {
				h.logger.Error("Failed to process scan results", zap.Error(err))
				return
			}

			callback(callbackResult)
		}
	}
}

func (h *Handler) filterDuplicates(configs []string) ([]string, error) {
	ctx := context.Background()
	uniqueConfigs := make([]string, 0, len(configs))
	pipe := h.redis.Pipeline()
	cmds := make([]*redis.IntCmd, len(configs))
	hashes := make([]string, len(configs))

	for i, config := range configs {
		hash := HashConfig(config)
		hashes[i] = hash
		cmds[i] = pipe.Exists(ctx, "config:"+hash)
	}

	if _, err := pipe.Exec(ctx); err != nil {
		h.logger.Error("Failed to filter duplicates", zap.Error(err))
		return nil, err
	}

	pipe = h.redis.Pipeline()
	for i, cmd := range cmds {
		if cmd.Val() == 0 {
			uniqueConfigs = append(uniqueConfigs, configs[i])
			pipe.Set(ctx, "config:"+hashes[i], "1", time.Hour)
		}
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		h.logger.Error("Failed to filter duplicates", zap.Error(err))
		return nil, err
	}
	return uniqueConfigs, err
}

func (h *Handler) Close() {
	h.workerPool.Stop()
}

// Helper functions
func writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(MessageResponse{
		Status:  code,
		Message: message,
	}); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}