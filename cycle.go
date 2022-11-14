package ctxdep

import (
	"context"
)

type cycle int

const cycleKey cycle = 0

type unlocker func()

type cycleChecker struct {
	inProcess map[*slot]bool
}

func enterSlotProcessing(ctx context.Context, s *slot) (context.Context, unlocker, error) {
	var checker *cycleChecker
	var checkerCtx context.Context
	c := ctx.Value(cycleKey)
	if c == nil {
		checker = &cycleChecker{
			inProcess: map[*slot]bool{},
		}
		checkerCtx = context.WithValue(ctx, cycleKey, checker)
	} else {
		checker = c.(*cycleChecker)
		checkerCtx = ctx
	}

	if _, found := checker.inProcess[s]; found {
		dc := GetDependencyContext(ctx)
		return nil, func() {}, &DependencyError{
			Message:        "cyclic dependency error getting slot",
			ReferencedType: s.slotType,
			Status:         dc.Status(),
		}
	}
	checker.inProcess[s] = true
	return checkerCtx, func() { delete(checker.inProcess, s) }, nil
}
