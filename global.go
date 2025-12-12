package ctxdep

import (
	"context"
	"fmt"
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
// This option will panic if used on a context whose parent is locked.
func WithOverrides() ContextOption {
	return func(dc *DependencyContext) {
		// Check if any parent context is locked
		parent := dc.parentContext
		for parent != nil {
			if pdc, ok := parent.(*DependencyContext); ok {
				if pdc.locked {
					panic("cannot use WithOverrides on a context with a locked parent")
				}
				parent = pdc.parentContext
			} else {
				// If we encounter a non-DependencyContext, check its value
				if val := parent.Value(dependencyContextKey); val != nil {
					if pdc, ok := val.(*DependencyContext); ok && pdc.locked {
						panic("cannot use WithOverrides on a context with a locked parent")
					}
				}
				break
			}
		}
		dc.loose = true
	}
}

// CleanupFunc represents a function that cleans up a dependency of type T.
type CleanupFunc[T any] func(T)

// WithCleanup enables cleanup functionality for the dependency context.
// When Cleanup(ctx) is called, dependencies implementing io.Closer will have their
// Close() method called. This must be used to enable any cleanup behavior.
// Cleanup is not automatic - you must explicitly call Cleanup(ctx) when done.
func WithCleanup() ContextOption {
	return func(dc *DependencyContext) {
		dc.cleanupEnabled = true
	}
}

// WithCleanupFunc registers a custom cleanup function for dependencies of type T.
// This automatically enables cleanup functionality if not already enabled.
// The cleanup function will be called when Cleanup(ctx) is explicitly called.
// Custom cleanup functions take precedence over automatic io.Closer cleanup.
func WithCleanupFunc[T any](cleanup CleanupFunc[T]) ContextOption {
	return func(dc *DependencyContext) {
		dc.cleanupEnabled = true
		var zero T
		cleanupType := reflect.TypeOf(&zero).Elem()
		dc.cleanupFuncs.Store(cleanupType, cleanup)
	}
}

// WithLock locks the dependency context, preventing any child contexts from using
// WithOverrides(). This is useful in production environments to ensure dependencies
// cannot be accidentally overridden. Dependencies marked with Overrideable() can
// still be overridden even in locked contexts.
func WithLock() ContextOption {
	return func(dc *DependencyContext) {
		dc.locked = true
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
func NewDependencyContext(ctx context.Context, args ...any) *DependencyContext {
	dc := &DependencyContext{
		parentContext: ctx,
		slots:         sync.Map{},
		cleanupFuncs:  sync.Map{},
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

	// Since DependencyContext now implements context.Context, we use it directly
	dc.selfContext = dc
	if err := dc.addDependenciesAndInitialize(dc, dependencies...); err != nil {
		panic(err.Error())
	}

	// No longer starting a monitoring goroutine - cleanup is now explicit

	return dc
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
func NewLooseDependencyContext(ctx context.Context, dependencies ...any) *DependencyContext {
	return NewDependencyContext(ctx, append([]any{WithOverrides()}, dependencies...)...)
}

// GetDependencyContext finds a DependencyContext in the context stack and returns it.
// if a DependencyContext is not found or is the wrong type then this function
// panics.
func GetDependencyContext(ctx context.Context) *DependencyContext {
	dc, err := GetDependencyContextWithError(ctx)
	if err != nil {
		panic(err.Error())
	}
	return dc
}

// GetDependencyContextWithError finds a DependencyContext in the context stack and returns it.
// If a DependencyContext is not found or is the wrong type, it returns an error instead of panicking.
func GetDependencyContextWithError(ctx context.Context) (*DependencyContext, error) {
	value := ctx.Value(dependencyContextKey)
	if value == nil {
		return nil, fmt.Errorf("no dependency context available")
	}
	dc, ok := value.(*DependencyContext)
	if !ok {
		// We should never get here.
		return nil, fmt.Errorf("dependency context unexpected type")
	}
	return dc, nil
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
// if the dependency is not found or if there is no dependency context.
func GetOptional[T any](ctx context.Context) (T, bool) {
	dc, err := GetDependencyContextWithError(ctx)
	if err != nil {
		var zero T
		return zero, false
	}
	var target T
	err = dc.FillDependency(ctx, &target)
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

// NewDependencyContextWithValidation creates a new dependency context like NewDependencyContext,
// but runs all registered validators before returning. If any validator returns an error,
// the context creation fails and returns that error.
//
// Validators are registered using the Validate() function:
//
//	ctx, err := NewDependencyContextWithValidation(parent,
//	    db,
//	    order,
//	    Validate(validateOrder),
//	)
//	if err != nil {
//	    // validation failed
//	}
//
// Validators are run after all dependencies are initialized but before any
// immediate processing occurs.
func NewDependencyContextWithValidation(ctx context.Context, args ...any) (*DependencyContext, error) {
	dc := &DependencyContext{
		parentContext: ctx,
		slots:         sync.Map{},
		cleanupFuncs:  sync.Map{},
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

	// Since DependencyContext now implements context.Context, we use it directly
	dc.selfContext = dc

	// Use the same initialization flow, but return the error instead of panicking
	if err := dc.addDependenciesAndInitialize(dc, dependencies...); err != nil {
		return nil, err
	}

	return dc, nil
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

// Lock locks the dependency context found in the given context, preventing any child
// contexts from using WithOverrides(). This is a convenience function that finds the
// DependencyContext and calls Lock() on it.
//
// This is useful in production code to ensure dependencies cannot be overridden:
//
//	ctx := setupApplication()
//	ctxdep.Lock(ctx)  // Lock the context for production use
func Lock(ctx context.Context) {
	dc := GetDependencyContext(ctx)
	dc.Lock()
}
