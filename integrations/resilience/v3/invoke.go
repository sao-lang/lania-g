// invoke.go 实现 resilience 集成统一的受保护调用入口。
package resilience

import (
	"fmt"
	"time"

	"github.com/sao-lang/lania-g/kernel/v3/aop"
)

var timeSleep = time.Sleep

func (s *Service) invoke(execCtx *aop.ExecutionContext, next aop.CallHandler, routeKey string) (any, error) {
	if s == nil {
		return next.Handle()
	}
	timeoutConfig := s.getTimeoutConfig(routeKey)
	if !timeoutConfig.Enabled || timeoutConfig.Duration <= 0 {
		return next.Handle()
	}
	type result struct {
		value any
		err   error
	}
	ch := make(chan result, 1)
	go func() {
		value, err := next.Handle()
		ch <- result{value: value, err: err}
	}()
	select {
	case out := <-ch:
		return out.value, out.err
	case <-time.After(timeoutConfig.Duration):
		return nil, fmt.Errorf("resilience timeout exceeded: %s", timeoutConfig.Duration)
	}
}
