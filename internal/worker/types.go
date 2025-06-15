package workerpool

import (
	"context"
	"time"
)

type Task struct {
	ID       string
	Data     interface{}
	Execute  func(ctx context.Context, data interface{}) (interface{}, error)
	Callback func(result interface{}, err error)
}

type Result struct {
	TaskID    string
	Result    interface{}
	Error     error
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
}

type BatchTask struct {
	Tasks []Task
}

type WorkerPoolConfig struct {
	WorkerCount   int
	TaskQueueSize int
}

type WorkerPoolStats struct {
	WorkerCount    int
	TotalTasks     int64
	CompletedTasks int64
	FailedTasks    int64
	QueueLength    int64
	Uptime         time.Duration
	IsRunning      bool
}
