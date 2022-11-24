package ctxdep

import (
	"context"
	"reflect"
)

type ContextUpdater func(ctx context.Context, depName string) context.Context

// immediateDependencies is an internal wrapper to signal to the DependencyContext
// that these dependencies should immediately be resolved. This is created by
// Immediate() or ImmediateCtxMutator()
type immediateDependencies struct {
	dependencies []any
	ctxUpdater   ContextUpdater
}

// Immediate is used to signal the DependencyContext to call the specified generators
// immediately to resolve the dependencies in a new goroutine.
func Immediate(deps ...any) *immediateDependencies {
	return &immediateDependencies{
		dependencies: deps,
	}
}

// ImmediateCtxMutator is used to signal the DependencyContext to call the specified
// generators immediately to resolve the dependencies in a new goroutine. This works
// the same as Immediate() except with the addition of a ContextUpdater. The
// ContextUpdater has a chance to, as the name suggests, update the passed in context
// and return the context to be used when running the generator. This can be critical
// in certain cases that require specific things on the context.
func ImmediateCtxMutator(ctxUpdater ContextUpdater, deps ...any) *immediateDependencies {
	return &immediateDependencies{
		dependencies: deps,
		ctxUpdater:   ctxUpdater,
	}
}

// resolveImmediateDependencies goes through all the slots on forces the generator
// to get run for each of the immediate slots.
func (d *DependencyContext) resolveImmediateDependencies(ctx context.Context) {
	// We can be nonchalant in calling all the slots at this time even if there
	// are multiple slots that are created by the same generator. Whichever one
	// gets called first will lock the slot and the other ones will block. When
	// they eventually unblocked the dependency will already have been resolved
	// so the generation will not get invoked again. The additional overhead is
	// the cost of creation of the extra goroutines and the locks.
	for _, slot := range d.slots {
		if slot.immediate != nil {
			threadCtx := ctx
			if slot.immediate.ctxUpdater != nil {
				threadCtx = slot.immediate.ctxUpdater(ctx, slot.slotType.String())
			}
			go func() {
				target := reflect.New(slot.slotType)
				err := d.getValue(threadCtx, slot, slot.slotType, target.Interface())
				if err != nil {
					// The best we can do is ignore this for now since we're
					// inside nested goroutines and the original call has returned.
					// By ignoring this error now, the dependency remains unset
					// and the call to fetch it will retry the call and either
					// succeed or (likely) fail again. The new failure will at
					// least be in a better place to report this though.
				}
			}()
		}
	}
}
