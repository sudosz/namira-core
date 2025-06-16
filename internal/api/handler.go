package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/NaMiraNet/rayping/internal/core"
	workerpool "github.com/NaMiraNet/rayping/internal/worker"
	"github.com/go-redis/redis/v8"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

type CallbackHandlerResult struct {
	JobID   string
	Results []core.CheckResult
	Error   error
}

type CallbackHandler func(CallbackHandlerResult)

type Handler struct {
	core       *core.Core
	workerPool *workerpool.WorkerPool
	redis      *redis.Client
	jobs       sync.Map
	logger     *zap.Logger
}

func NewHandler(c *core.Core, redisClient *redis.Client, callbackHandler CallbackHandler, logger *zap.Logger) *Handler {
	pool := workerpool.NewWorkerPool(workerpool.WorkerPoolConfig{
		WorkerCount:   5,
		TaskQueueSize: 100,
	})

	handler := &Handler{
		core:       c,
		workerPool: pool,
		redis:      redisClient,
		logger:     logger,
	}

	pool.SetResultHandler(handler.handleTaskResult(callbackHandler))
	if err := pool.Start(); err != nil {
		panic("Failed to start worker pool: " + err.Error())
	}
	return handler
}

func (h *Handler) handleScan(w http.ResponseWriter, r *http.Request) {
	var req ScanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Invalid Json", zap.Error(err))
		writeError(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if len(req.Configs) == 0 {
		h.logger.Error("No configs provided")
		writeError(w, "No configs provided", http.StatusBadRequest)
		return
	}

	uniqueConfigs, err := h.filterDuplicates(req.Configs)
	if err != nil {
		h.logger.Error("Failed to filter duplicates", zap.Error(err))
		writeError(w, "Failed to filter duplicates", http.StatusInternalServerError)
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
		status := "error"
		checkResult := CheckResult{
			Index:  i,
			Status: string(result.Status),
			Delay:  result.RealDelay.Milliseconds(),
		}

		if result.Error != nil {
			checkResult.Error = result.Error.Error()
			h.logger.Error("Error in check", zap.String("error", result.Error.Error()))
			job.AddResult(status, checkResult)
		} else {
			h.logger.Info("Check result", zap.String("config", result.Raw))
			job.AddResult(HashConfig(result.Raw), checkResult)
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
			callback(result.Result.(CallbackHandlerResult))
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
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(MessageResponse{
		Status:  code,
		Message: message,
	})
	w.WriteHeader(code)
}
