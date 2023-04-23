package ctxdep

import (
	"context"
	"reflect"
	"sync"
)

type cycle int

const cycleKey cycle = 0

type unlocker func()

// cycleChecker detects cyclic dependencies in DependencyContext by tracking
// inProcess generator types and managing concurrent access.
type cycleChecker struct {
	inProcess map[reflect.Type]bool
	lock      sync.Mutex
}

// enterSlotProcessing detects cyclic dependencies while processing a slot in the DependencyContext.
// It returns an updated context, an unlocker function, and an error if a cycle is found.
func (d *DependencyContext) enterSlotProcessing(ctx context.Context, s *slot) (context.Context, unlocker, error) {
	var checker *cycleChecker
	var checkerCtx context.Context
	c := ctx.Value(cycleKey)

	// Check if cycleChecker exists in the context
	if c == nil {
		// Create a new cycleChecker and add it to the context
		checker = &cycleChecker{
			inProcess: map[reflect.Type]bool{},
		}
		checkerCtx = context.WithValue(ctx, cycleKey, checker)
	} else {
		// Use the existing cycleChecker from the context
		checker = c.(*cycleChecker)
		checkerCtx = ctx
	}

	genType := reflect.TypeOf(s.generator)
	checker.lock.Lock()
	defer checker.lock.Unlock()

	// Check if the generator type is already in the inProcess map (cyclic dependency)
	if _, found := checker.inProcess[genType]; found {
		return nil, func() {}, &DependencyError{
			Message:        "cyclic dependency error getting slot",
			ReferencedType: s.slotType,
			Status:         d.Status(),
		}
	}

	// Mark the generator type as inProcess
	checker.inProcess[genType] = true

	return checkerCtx, func() {
		checker.lock.Lock()
		// Remove the generator type from the inProcess map
		delete(checker.inProcess, genType)
		checker.lock.Unlock()
	}, nil
}
