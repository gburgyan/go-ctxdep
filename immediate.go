package ctxdep

import (
	"context"
	"github.com/gburgyan/go-timing"
	"log"
	"reflect"
)

// immediateDependencies is an internal wrapper to signal to the DependencyContext
// that these dependencies should immediately be resolved. This is created by
// Immediate() or ImmediateCtxMutator()
type immediateDependencies struct {
	dependencies []any
}

// Immediate is used to signal the DependencyContext to call the specified generators
// immediately to resolve the dependencies in a new goroutine.
func Immediate(deps ...any) *immediateDependencies {
	return &immediateDependencies{
		dependencies: deps,
	}
}

// resolveImmediateDependencies goes through all the slots on forces the generator
// to get run for each of the immediate slots.
func (d *DependencyContext) resolveImmediateDependencies(ctx context.Context) {
	// Check if there are any immediate dependencies. If not, we can return early.
	anyImmediate := false
	d.slots.Range(func(_, sa any) bool {
		slot := sa.(*slot)
		if slot.immediate != nil {
			anyImmediate = true
			return false
		}
		return true
	})
	if !anyImmediate {
		return
	}

	var effectiveContext context.Context
	if EnableTiming >= TimingImmediate {
		tCtx := timing.ForName(ctx, "ImmediateDeps")
		tCtx.Async = true
		effectiveContext = tCtx
	} else {
		effectiveContext = ctx
	}

	// We can be nonchalant in calling all the slots at this time even if there
	// are multiple slots that are created by the same generator. Whichever one
	// gets called first will lock the slot and the other ones will block. When
	// they eventually unblocked the dependency will already have been resolved
	// so the generation will not get invoked again. The additional overhead is
	// the cost of creation of the extra goroutines and the locks.
	d.slots.Range(func(_, sa any) bool {
		slot := sa.(*slot)
		if slot.immediate != nil {
			go func() {
				defer func() {
					// Catch panics
					if r := recover(); r != nil {
						// The best we can do is ignore this for now since we're
						// inside nested goroutines and the original call has returned.
						// By ignoring this error now, the dependency remains unset
						// and the call to fetch it will retry the call and either
						// succeed or (likely) fail again. The new failure will at
						// least be in a better place to report this though.
						log.Printf("panic resolving immediate dependency for %v: %v", slot.slotType, r)
					}
				}()
				target := reflect.New(slot.slotType)
				err := d.getValue(effectiveContext, slot, slot.slotType, target.Interface())
				if err != nil {
					// The best we can do is ignore this for now since we're
					// inside nested goroutines and the original call has returned.
					// By ignoring this error now, the dependency remains unset
					// and the call to fetch it will retry the call and either
					// succeed or (likely) fail again. The new failure will at
					// least be in a better place to report this though.
					log.Printf("error resolving immediate dependency: %v", err)
				}
			}()
		}
		return true
	})
}
