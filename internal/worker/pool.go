package workerpool

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type WorkerPool struct {
	workers     []*Worker
	taskQueue   chan Task
	resultQueue chan Result
	workerCount int
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	started     bool
	mu          sync.RWMutex

	// Metrics
	totalTasks     int64
	completedTasks int64
	failedTasks    int64
	startTime      time.Time
}

// NewWorkerPool creates a new worker pool with the specified configuration
func NewWorkerPool(config WorkerPoolConfig) *WorkerPool {
	if config.WorkerCount <= 0 {
		config.WorkerCount = 5
	}
	if config.TaskQueueSize <= 0 {
		config.TaskQueueSize = 100
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &WorkerPool{
		workers:     make([]*Worker, 0, config.WorkerCount),
		taskQueue:   make(chan Task, config.TaskQueueSize),
		resultQueue: make(chan Result, config.TaskQueueSize),
		workerCount: config.WorkerCount,
		ctx:         ctx,
		cancel:      cancel,
		startTime:   time.Now(),
	}
}

func (wp *WorkerPool) Start() error {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if wp.started {
		return fmt.Errorf("worker pool is already started")
	}

	// start workers
	for i := 0; i < wp.workerCount; i++ {
		worker := &Worker{
			ID:         i + 1,
			taskChan:   wp.taskQueue,
			resultChan: wp.resultQueue,
			quit:       make(chan bool),
			wg:         &wp.wg,
		}
		wp.workers = append(wp.workers, worker)
		wp.wg.Add(1)
		go worker.start(wp.ctx)
	}

	wp.started = true
	return nil
}

func (wp *WorkerPool) Submit(task Task) error {
	wp.mu.RLock()
	defer wp.mu.RUnlock()

	if !wp.started {
		return fmt.Errorf("worker pool is not started")
	}

	select {
	case wp.taskQueue <- task:
		wp.totalTasks++
		return nil
	case <-wp.ctx.Done():
		return fmt.Errorf("worker pool is shutting down")
	default:
		return fmt.Errorf("task queue is full")
	}
}

func (wp *WorkerPool) SubmitBatch(batch BatchTask) error {
	wp.mu.RLock()
	defer wp.mu.RUnlock()

	if !wp.started {
		return fmt.Errorf("worker pool is not started")
	}

	for _, task := range batch.Tasks {
		select {
		case wp.taskQueue <- task:
			wp.totalTasks++
		case <-wp.ctx.Done():
			return fmt.Errorf("worker pool is shutting down")
		default:
			return fmt.Errorf("task queue is full")
		}
	}

	return nil
}

func (wp *WorkerPool) Results() <-chan Result {
	return wp.resultQueue
}

func (wp *WorkerPool) Stop() {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if !wp.started {
		return
	}

	wp.cancel()
	close(wp.taskQueue)
	wp.wg.Wait()
	close(wp.resultQueue)

	wp.started = false
}

func (wp *WorkerPool) GetStats() WorkerPoolStats {
	wp.mu.RLock()
	defer wp.mu.RUnlock()

	return WorkerPoolStats{
		WorkerCount:    wp.workerCount,
		TotalTasks:     wp.totalTasks,
		CompletedTasks: wp.completedTasks,
		FailedTasks:    wp.failedTasks,
		QueueLength:    int64(len(wp.taskQueue)),
		Uptime:         time.Since(wp.startTime),
		IsRunning:      wp.started,
	}
}

func (wp *WorkerPool) WaitForCompletion(timeout time.Duration) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	timeoutChan := time.After(timeout)

	for {
		select {
		case <-ticker.C:
			stats := wp.GetStats()
			if stats.QueueLength == 0 && stats.CompletedTasks+stats.FailedTasks == stats.TotalTasks {
				return nil
			}
		case <-timeoutChan:
			return fmt.Errorf("timeout waiting for task completion")
		case <-wp.ctx.Done():
			return fmt.Errorf("worker pool was stopped")
		}
	}
}

func (wp *WorkerPool) SetResultHandler(handler func(Result)) {
	go func() {
		for result := range wp.resultQueue {
			wp.mu.Lock()
			if result.Error != nil {
				wp.failedTasks++
			} else {
				wp.completedTasks++
			}
			wp.mu.Unlock()

			if handler != nil {
				handler(result)
			}
		}
	}()
}
