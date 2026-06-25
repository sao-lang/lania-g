// strategy.go 定义 resilience 集成使用的策略接口与策略选择逻辑。
package resilience

func (s *Service) getTimeoutConfig(routeKey string) TimeoutConfig {
	if s == nil {
		return TimeoutConfig{}
	}
	if config, ok := s.config.Strategy.Timeout[routeKey]; ok {
		return config
	}
	return s.config.Timeout
}

func (s *Service) getRetryConfig(routeKey string) RetryConfig {
	if s == nil {
		return RetryConfig{}
	}
	if config, ok := s.config.Strategy.Retry[routeKey]; ok {
		return config
	}
	return s.config.Retry
}
