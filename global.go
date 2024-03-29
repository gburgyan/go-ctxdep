package ctxdep

import (
	"context"
	"sync"
)

type TimingMode int

const (
	// TimingDisable will disable timing for all contexts.
	TimingDisable TimingMode = iota

	// TimingImmediate will start a new placeholder context for actions that are taken
	// during the immediate dependency resolution phase (if any).
	TimingImmediate

	// TimingGenerators will start timing context for each generator that is called. This is useful
	// to see where all time of execution is being spent. It can also be helpful to see the exact stack
	// for the dependency resolution.
	TimingGenerators
)

var EnableTiming = TimingDisable

// NewDependencyContext adds a new dependency context to the context stack and returns
// the new context. It also adds any dependencies that are also passed in to the new
// dependency context. For a further discussion on what dependencies do and how
// they work, look at the documentation for DependencyContext. This applies strict
// evaluation and will not allow multiple dependencies to ever fill a slot. If there
// are multiple concrete types or generators can can fill a slot, this function
// will `panic`.
func NewDependencyContext(ctx context.Context, dependencies ...any) context.Context {
	dc := &DependencyContext{
		parentContext: ctx,
		slots:         sync.Map{},
	}
	newContext := context.WithValue(ctx, dependencyContextKey, dc)
	dc.selfContext = newContext
	dc.addDependenciesAndInitialize(newContext, dependencies...)
	return newContext
}

// NewLooseDependencyContext adds a new dependency context to the context stack and returns
// the new context. It also adds any dependencies that are also passed in to the new
// dependency context. For a further discussion on what dependencies do and how
// they work, look at the documentation for DependencyContext. This operates the same as
// NewDependencyContext except that it allows for overrides of existing dependencies. In case
// there are multiple dependencies that can fill a slot, the last concrete slot value will
// be used. In case there is no concrete value, the last generator will win.
func NewLooseDependencyContext(ctx context.Context, dependencies ...any) context.Context {
	dc := &DependencyContext{
		parentContext: ctx,
		slots:         sync.Map{},
		loose:         true,
	}
	newContext := context.WithValue(ctx, dependencyContextKey, dc)
	dc.selfContext = newContext
	dc.addDependenciesAndInitialize(newContext, dependencies...)
	return newContext
}

// GetDependencyContext finds a DependencyContext in the context stack and returns it.
// if a DependencyContext is not found or is the wrong type then this function
// panics.
func GetDependencyContext(ctx context.Context) *DependencyContext {
	value := ctx.Value(dependencyContextKey)
	if value == nil {
		panic("no dependency context available")
	}
	dc, ok := value.(*DependencyContext)
	if !ok {
		// We should never get here.
		panic("dependency context unexpected type")
	}
	return dc
}

// GetBatch behaves like GetBatchWithError except it will panic if the requested dependencies are not
// found. The typical behavior for a dependency that is not found is returning an error or
// panicking on the caller's side, so this presents a simplified interface for getting the
// required dependencies.
func GetBatch(ctx context.Context, target ...any) {
	err := GetBatchWithError(ctx, target...)
	if err != nil {
		panic(err)
	}
}

// Get returns the value of type T from the dependency context. It otherwise behaves exactly like
// GetBatch, but it only has the capability of returning a single value.
func Get[T any](ctx context.Context) T {
	dc := GetDependencyContext(ctx)
	var target T
	err := dc.FillDependency(ctx, &target)
	if err != nil {
		panic(err)
	}
	return target
}

// GetBatchWithError will try to get the requested dependencies from the context's
// DependencyContext. If it fails to do so it will return an error. If the context's
// DependencyContext is not found, this will still panic as its preconditions were
// not met. Similarly, if the target isn't a pointer to something, that will also trigger
// a panic.
func GetBatchWithError(ctx context.Context, target ...any) error {
	dc := GetDependencyContext(ctx)
	return dc.GetBatchWithError(ctx, target...)
}

// GetWithError returns the value of type T from the dependency context. It otherwise behaves exactly like
// GetBatchWithError, but it only has the capability of returning a single value and an error object.
func GetWithError[T any](ctx context.Context) (T, error) {
	dc := GetDependencyContext(ctx)
	var target T
	err := dc.FillDependency(ctx, &target)
	return target, err
}

// Status is a diagnostic tool that returns a string describing the state of the dependency
// context. The result is each dependency type that is known about, and if it has a value
// and if it has a generator that is capable of making that value.
//
// Note that while everything that is returned is true, if a type implements an interface
// or can be cast to another type, and that type hasn't been asked for yet, the other
// type is not yet known.
func Status(ctx context.Context) string {
	dc := GetDependencyContext(ctx)
	return dc.Status()
}
