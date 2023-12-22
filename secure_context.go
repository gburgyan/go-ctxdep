package ctxdep

import (
	"context"
	"github.com/gburgyan/go-timing"
	"time"
)

// secureContext is context object that returns values from the baseContext and timing information
// from the timingContext. The exception to this is that cycleKey comes from the timingContext which is
// used to check for and prevent cyclic dependencies.
//
// The reason this exists is to provide extra security around accessing the context and preventing
// accidental mixing of context information.
type secureContext struct {
	// baseContext is the context that contains the dependency information. This is the context
	// that existed when the dependency was created.
	baseContext context.Context

	// timingContext is the context that contains the timing information. This is the context
	// that existed when the dependency was requested.
	timingContext context.Context
}

func (h *secureContext) Deadline() (deadline time.Time, ok bool) {
	return h.timingContext.Deadline()
}

func (h *secureContext) Done() <-chan struct{} {
	return h.timingContext.Done()
}

func (h *secureContext) Err() error {
	return h.timingContext.Err()
}

func (h *secureContext) Value(key any) any {
	if key == cycleKey || key == timing.ContextTimingKey {
		return h.timingContext.Value(key)
	}
	return h.baseContext.Value(key)
}
