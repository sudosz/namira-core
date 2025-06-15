package workerpool

import (
	"context"
	"sync"
	"time"
)

type Worker struct {
	ID         int
	taskChan   chan Task
	resultChan chan Result
	quit       chan bool
	wg         *sync.WaitGroup
}

func (w *Worker) start(ctx context.Context) {
	defer w.wg.Done()

	for {
		select {
		case task, ok := <-w.taskChan:
			if !ok {
				return
			}
			w.executeTask(ctx, task)

		case <-ctx.Done():
			return
		}
	}
}

func (w *Worker) executeTask(ctx context.Context, task Task) {
	startTime := time.Now()
	taskCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	result, err := task.Execute(taskCtx, task.Data)

	endTime := time.Now()
	duration := endTime.Sub(startTime)

	taskResult := Result{
		TaskID:    task.ID,
		Result:    result,
		Error:     err,
		StartTime: startTime,
		EndTime:   endTime,
		Duration:  duration,
	}

	select {
	case w.resultChan <- taskResult:
	case <-ctx.Done():
		return
	}
	if task.Callback != nil {
		go task.Callback(result, err)
	}
}
