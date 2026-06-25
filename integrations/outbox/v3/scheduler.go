// scheduler.go 实现 outbox 集成与调度器之间的协作逻辑。
package outbox

import (
	"context"
	"time"
)

// NewScheduler 基于 service 和调度配置创建一个调度器，并立即启动后台轮询。
func NewScheduler(service *Service, config SchedulerConfig) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	scheduler := &Scheduler{
		service: service,
		config:  config,
		ctx:     ctx,
		cancel:  cancel,
	}

	scheduler.Start()
	return scheduler
}

// Start 启动后台轮询协程。
func (s *Scheduler) Start() {
	s.wg.Add(1)
	go s.run()
}

// Stop 停止后台轮询并等待退出。
func (s *Scheduler) Stop() {
	s.cancel()
	s.wg.Wait()
}

func (s *Scheduler) run() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.processBatch()
		}
	}
}

func (s *Scheduler) processBatch() {
	ctx, cancel := context.WithTimeout(s.ctx, s.config.Interval)
	defer cancel()

	s.service.Flush(ctx, s.config.BatchSize)
}
