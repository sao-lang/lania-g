package events

import (
	stdctx "context"
	"sync/atomic"

)

type eventBridgeService struct {
	count atomic.Int32
}

func (s *eventBridgeService) HandleUserCreated(ctx stdctx.Context, name string) error {
	_ = ctx
	if name != "alice" {
		return stdctx.Canceled
	}
	s.count.Add(1)
	return nil
}

