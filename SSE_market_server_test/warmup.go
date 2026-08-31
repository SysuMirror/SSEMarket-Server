package cache

import (
	"context"
	"log"
	"sort"
)

// WarmupTask 启动预热任务。
type WarmupTask struct {
	Name     string
	Priority int
	Load     func(ctx context.Context) error
}

// RunWarmup 按优先级执行预热任务；单个失败不阻塞其余任务。
func RunWarmup(ctx context.Context, tasks []WarmupTask) {
	sort.Slice(tasks, func(i, j int) bool {
		if tasks[i].Priority != tasks[j].Priority {
			return tasks[i].Priority < tasks[j].Priority
		}
		return tasks[i].Name < tasks[j].Name
	})
	log.Printf("[cache] warmup starting, %d tasks", len(tasks))
	for _, t := range tasks {
		if ctx.Err() != nil {
			log.Printf("[cache] warmup aborted: %v", ctx.Err())
			return
		}
		if err := t.Load(ctx); err != nil {
			log.Printf("[cache] warmup failed task=%s err=%v", t.Name, err)
		} else {
			log.Printf("[cache] warmup ok task=%s", t.Name)
		}
	}
}
