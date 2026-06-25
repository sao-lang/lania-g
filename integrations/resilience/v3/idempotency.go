// idempotency.go 实现 resilience 集成的幂等控制能力。
package resilience

import "github.com/sao-lang/lania-g/kernel/v3/runtime"

func (s *Service) tryReplay(hc *runtime.HandlerContext) (bool, any, error) {
	if s == nil || hc == nil {
		return false, nil, nil
	}
	routeKey := routeKeyOf(hc)
	idempotencyConfig := s.getIdempotencyConfig(routeKey)
	if !idempotencyConfig.Enabled {
		return false, nil, nil
	}
	key := firstHeader(hc, idempotencyConfig.Header)
	if key == "" {
		return false, nil, nil
	}
	now := s.config.Clock()
	s.mu.Lock()
	defer s.mu.Unlock()
	record := s.idempotency[key]
	if record == nil || now.After(record.expiresAt) {
		delete(s.idempotency, key)
		return false, nil, nil
	}
	hc.Response.Status = record.response.Status
	hc.Response.Body = record.response.Body
	hc.Response.Headers = cloneHeaders(record.response.Headers)
	hc.Set(MetadataKeyIdempotencyRecord, key)
	return true, record.response.Body, nil
}

func (s *Service) storeReplay(hc *runtime.HandlerContext, result any) {
	if s == nil || hc == nil {
		return
	}
	routeKey := routeKeyOf(hc)
	idempotencyConfig := s.getIdempotencyConfig(routeKey)
	if !idempotencyConfig.Enabled {
		return
	}
	key := firstHeader(hc, idempotencyConfig.Header)
	if key == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.idempotency[key] = &idempotentRecord{
		expiresAt: s.config.Clock().Add(idempotencyConfig.TTL),
		response: responseSnapshot{
			Status:  hc.Response.Status,
			Headers: cloneHeaders(hc.Response.Headers),
			Body:    result,
		},
	}
}

func (s *Service) getIdempotencyConfig(routeKey string) IdempotencyConfig {
	if s == nil {
		return IdempotencyConfig{}
	}
	if config, ok := s.config.Strategy.Idempotency[routeKey]; ok {
		return config
	}
	return s.config.Idempotency
}
