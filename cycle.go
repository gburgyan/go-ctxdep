package ctxdep

import (
	"context"
	"reflect"
	"sync"
)

type cycle int

const cycleKey cycle = 0

type unlocker func()

type cycleChecker struct {
	inProcess map[reflect.Type]bool
	lock      sync.Mutex
}

func (d *DependencyContext) enterSlotProcessing(ctx context.Context, s *slot) (context.Context, unlocker, error) {
	var checker *cycleChecker
	var checkerCtx context.Context
	c := ctx.Value(cycleKey)
	if c == nil {
		checker = &cycleChecker{
			inProcess: map[reflect.Type]bool{},
		}
		checkerCtx = context.WithValue(ctx, cycleKey, checker)
	} else {
		checker = c.(*cycleChecker)
		checkerCtx = ctx
	}

	genType := reflect.TypeOf(s.generator)
	checker.lock.Lock()
	defer checker.lock.Unlock()

	if _, found := checker.inProcess[genType]; found {
		return nil, func() {}, &DependencyError{
			Message:        "cyclic dependency error getting slot",
			ReferencedType: s.slotType,
			Status:         d.Status(),
		}
	}
	checker.inProcess[genType] = true

	return checkerCtx, func() {
		checker.lock.Lock()
		delete(checker.inProcess, genType)
		checker.lock.Unlock()
	}, nil
}
