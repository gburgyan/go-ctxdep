package ctxdep

import (
	"context"
	"time"
)

// secureContext is context object that returns values from the baseContext and timing information
// from the innerContext. The exception to this is that cycleKey comes from teh innerContext which
// used to check for and prevent cyclic dependencies.
//
// The reason this exists is to provide extra security around accessing the context and preventing
// accidental mixing of context information.
type secureContext struct {
	baseContext  context.Context
	innerContext context.Context
}

func (h *secureContext) Deadline() (deadline time.Time, ok bool) {
	return h.innerContext.Deadline()
}

func (h *secureContext) Done() <-chan struct{} {
	return h.innerContext.Done()
}

func (h *secureContext) Err() error {
	return h.innerContext.Err()
}

func (h *secureContext) Value(key any) any {
	if key == cycleKey {
		return h.innerContext.Value(key)
	}
	return h.baseContext.Value(key)
}
