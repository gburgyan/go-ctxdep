package ctxdep

import (
	"context"
	"reflect"
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

// ContextOption is a functional option for configuring a DependencyContext.
type ContextOption func(*DependencyContext)

// WithOverrides allows dependencies to override existing ones. When this option is used,
// if there are multiple dependencies that can fill a slot, the last concrete slot value
// will be used. In case there is no concrete value, the last generator will win.
// This is useful for testing scenarios where you want to override specific dependencies.
func WithOverrides() ContextOption {
	return func(dc *DependencyContext) {
		dc.loose = true
	}
}

// CleanupFunc represents a function that cleans up a dependency of type T.
type CleanupFunc[T any] func(T)

// WithCleanup enables cleanup functionality for the dependency context.
// When the context is cancelled, dependencies implementing io.Closer will have their
// Close() method called automatically. This must be called to enable any cleanup behavior.
func WithCleanup() ContextOption {
	return func(dc *DependencyContext) {
		dc.cleanupEnabled = true
	}
}

// WithCleanupFunc registers a custom cleanup function for dependencies of type T.
// This automatically enables cleanup functionality if not already enabled.
// The cleanup function will be called when the context is cancelled.
// Custom cleanup functions take precedence over automatic io.Closer cleanup.
func WithCleanupFunc[T any](cleanup CleanupFunc[T]) ContextOption {
	return func(dc *DependencyContext) {
		dc.cleanupEnabled = true
		var zero T
		cleanupType := reflect.TypeOf(&zero).Elem()
		dc.cleanupFuncs.Store(cleanupType, cleanup)
	}
}

// NewDependencyContext adds a new dependency context to the context stack and returns
// the new context. It also adds any dependencies that are also passed in to the new
// dependency context. For a further discussion on what dependencies do and how
// they work, look at the documentation for DependencyContext.
//
// By default, this applies strict evaluation and will not allow multiple dependencies
// to ever fill a slot. If there are multiple concrete types or generators that can
// fill a slot, this function will `panic`. Use WithOverrides() option to allow
// overriding existing dependencies.
//
// Options and dependencies can be mixed in any order. Options are applied first,
// then dependencies are added.
func NewDependencyContext(ctx context.Context, args ...any) context.Context {
	dc := &DependencyContext{
		parentContext: ctx,
		slots:         sync.Map{},
	}

	// Separate options from dependencies
	var options []ContextOption
	var dependencies []any

	for _, arg := range args {
		if opt, ok := arg.(ContextOption); ok {
			options = append(options, opt)
		} else {
			dependencies = append(dependencies, arg)
		}
	}

	// Apply options
	for _, opt := range options {
		opt(dc)
	}

	newContext := context.WithValue(ctx, dependencyContextKey, dc)
	dc.selfContext = newContext
	dc.addDependenciesAndInitialize(newContext, dependencies...)

	// Set up cleanup monitoring only if enabled
	if dc.cleanupEnabled {
		go dc.monitorContextDone(newContext)
	}

	return newContext
}

// NewLooseDependencyContext adds a new dependency context to the context stack and returns
// the new context. It also adds any dependencies that are also passed in to the new
// dependency context. For a further discussion on what dependencies do and how
// they work, look at the documentation for DependencyContext. This operates the same as
// NewDependencyContext except that it allows for overrides of existing dependencies. In case
// there are multiple dependencies that can fill a slot, the last concrete slot value will
// be used. In case there is no concrete value, the last generator will win.
//
// Deprecated: Use NewDependencyContext with WithOverrides() option instead.
func NewLooseDependencyContext(ctx context.Context, dependencies ...any) context.Context {
	return NewDependencyContext(ctx, append([]any{WithOverrides()}, dependencies...)...)
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

// GetOptional returns the value of type T from the dependency context along with a boolean
// indicating whether the dependency was found. Unlike Get, this function does not panic
// if the dependency is not found.
func GetOptional[T any](ctx context.Context) (T, bool) {
	dc := GetDependencyContext(ctx)
	var target T
	err := dc.FillDependency(ctx, &target)
	if err != nil {
		return target, false
	}
	return target, true
}

// GetBatchOptional behaves like GetBatch but returns a slice of booleans indicating
// which dependencies were successfully filled. It does not panic if dependencies are not found.
func GetBatchOptional(ctx context.Context, target ...any) []bool {
	dc := GetDependencyContext(ctx)
	results := make([]bool, len(target))
	for i, t := range target {
		err := dc.FillDependency(ctx, t)
		results[i] = err == nil
	}
	return results
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
