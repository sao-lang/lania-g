// rate_limit.go 实现 resilience 集成的限流能力。
package resilience

import (
	"fmt"

	"github.com/sao-lang/lania-g/kernel/v3/runtime"
)

func (s *Service) allowRate(hc *runtime.HandlerContext) error {
	if s == nil || hc == nil {
		return nil
	}
	rateLimit := s.getRateLimitConfig(hc.RouteKey)
	if !rateLimit.Enabled {
		return nil
	}
	key := hc.RouteKey + ":" + firstHeader(hc, rateLimit.Header)
	if key == ":" {
		key = hc.RouteKey
	}
	if s.distributed != nil {
		return s.allowRateDistributed(key, rateLimit)
	}
	now := s.config.Clock()
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.rateWindows[key]
	if state == nil || now.Sub(state.start) >= rateLimit.Window {
		s.rateWindows[key] = &rateState{start: now, count: 1}
		return nil
	}
	if state.count >= rateLimit.Limit {
		return fmt.Errorf("rate limit exceeded for %s", key)
	}
	state.count++
	return nil
}

func (s *Service) allowRateDistributed(key string, config RateLimitConfig) error {
	now := s.config.Clock()
	windowKey := key + ":" + now.Truncate(config.Window).String()
	countStr, err := s.distributed.Get(windowKey)
	if err != nil {
		return nil
	}
	count := 0
	if countStr != "" {
		fmt.Sscanf(countStr, "%d", &count)
	}
	if count >= config.Limit {
		return fmt.Errorf("rate limit exceeded for %s", key)
	}
	s.distributed.Set(windowKey, fmt.Sprintf("%d", count+1), config.Window)
	return nil
}

func (s *Service) getRateLimitConfig(routeKey string) RateLimitConfig {
	if s == nil {
		return RateLimitConfig{}
	}
	if config, ok := s.config.Strategy.RateLimit[routeKey]; ok {
		return config
	}
	return s.config.RateLimit
}
