// circuit.go 实现 resilience 集成的熔断能力。
package resilience

import (
	"fmt"
	"time"
)

func (s *Service) guardBreaker(routeKey string) error {
	if s == nil {
		return nil
	}
	circuit := s.getCircuitConfig(routeKey)
	if !circuit.Enabled {
		return nil
	}
	if s.distributed != nil {
		return s.guardBreakerDistributed(routeKey, circuit)
	}
	now := s.config.Clock()
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.breakers[routeKey]
	if state != nil && now.Before(state.openedUntil) {
		return fmt.Errorf("circuit breaker is open for %s", routeKey)
	}
	return nil
}

func (s *Service) onFailure(routeKey string) {
	if s == nil {
		return
	}
	circuit := s.getCircuitConfig(routeKey)
	if !circuit.Enabled {
		return
	}
	if s.distributed != nil {
		s.onFailureDistributed(routeKey, circuit)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.breakers[routeKey]
	if state == nil {
		state = &breakerState{}
		s.breakers[routeKey] = state
	}
	state.failures++
	if state.failures >= circuit.FailureThreshold {
		state.openedUntil = s.config.Clock().Add(circuit.OpenTimeout)
		if s.distributed != nil {
			s.persistBreakerState(routeKey, state)
		}
	}
}

func (s *Service) onSuccess(routeKey string) {
	if s == nil {
		return
	}
	if s.distributed != nil {
		s.onSuccessDistributed(routeKey)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.breakers, routeKey)
}

func (s *Service) guardBreakerDistributed(routeKey string, config CircuitBreakerConfig) error {
	state, err := s.loadBreakerState(routeKey)
	if err == nil && state != nil {
		now := s.config.Clock()
		if now.Before(state.openedUntil) {
			return fmt.Errorf("circuit breaker is open for %s", routeKey)
		}
	}
	return nil
}

func (s *Service) onFailureDistributed(routeKey string, config CircuitBreakerConfig) {
	state, err := s.loadBreakerState(routeKey)
	if err != nil {
		state = &breakerState{}
	}
	state.failures++
	if state.failures >= config.FailureThreshold {
		state.openedUntil = s.config.Clock().Add(config.OpenTimeout)
	}
	s.persistBreakerState(routeKey, state)
}

func (s *Service) onSuccessDistributed(routeKey string) {
	s.distributed.Delete("circuit:" + routeKey)
}

func (s *Service) loadBreakerState(routeKey string) (*breakerState, error) {
	if s.distributed == nil {
		return nil, fmt.Errorf("no distributed store")
	}
	data, err := s.distributed.Get("circuit:" + routeKey)
	if err != nil || data == "" {
		return nil, err
	}
	state := &breakerState{}
	var unixTime int64
	fmt.Sscanf(data, "%d,%d", &state.failures, &unixTime)
	state.openedUntil = time.Unix(unixTime, 0)
	return state, nil
}

func (s *Service) persistBreakerState(routeKey string, state *breakerState) error {
	if s.distributed == nil || state == nil {
		return nil
	}
	data := fmt.Sprintf("%d,%d", state.failures, state.openedUntil.Unix())
	return s.distributed.Set("circuit:"+routeKey, data, state.openedUntil.Sub(s.config.Clock()))
}

func (s *Service) getCircuitConfig(routeKey string) CircuitBreakerConfig {
	if s == nil {
		return CircuitBreakerConfig{}
	}
	if config, ok := s.config.Strategy.Circuit[routeKey]; ok {
		return config
	}
	return s.config.Circuit
}
