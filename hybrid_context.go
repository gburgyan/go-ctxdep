package ctxdep

import (
	"context"
	"time"
)

type hybridContext struct {
	valueContext  context.Context
	timingContext context.Context
}

func (h *hybridContext) Deadline() (deadline time.Time, ok bool) {
	return h.timingContext.Deadline()
}

func (h *hybridContext) Done() <-chan struct{} {
	return h.timingContext.Done()
}

func (h *hybridContext) Err() error {
	return h.timingContext.Err()
}

func (h *hybridContext) Value(key any) any {
	return h.valueContext.Value(key)
}
