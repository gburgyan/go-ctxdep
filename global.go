package ctxdep

import (
	"context"
	"reflect"
)

// NewDependencyContext adds a new dependency context to the context stack and returns
// the new context. It also adds any dependencies that are also passed in to the new
// dependency context. For a further discussion on what dependencies do and how
// they work, look at the documentation for DependencyContext.
func NewDependencyContext(ctx context.Context, dependencies ...interface{}) context.Context {
	dc := &DependencyContext{
		parentContext: ctx,
		slots:         map[reflect.Type]*slot{},
	}
	newContext := context.WithValue(ctx, dependencyContextKey, dc)
	dc.AddDependencies(newContext, dependencies...)
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

// Get behaves like GetWithError except it will panic if the requested dependencies are not
// found. The typical behavior for a dependency that is not found is returning an error or
// panicking on the caller's side, so this presents a simplified interface for getting the
// required dependencies.
func Get(ctx context.Context, target ...interface{}) {
	err := GetWithError(ctx, target...)
	if err != nil {
		panic(err)
	}
}

// GetWithError will try to get the requested dependencies from the context's
// DependencyContext. If it fails to do so it will return an error. If the context's
// DependencyContext is not found, this will still panic as its preconditions were
// not met. Similarly, if the target isn't a pointer to something, that will also trigger
// a panic.
func GetWithError(ctx context.Context, target ...interface{}) error {
	dc := GetDependencyContext(ctx)
	return dc.GetWithError(ctx, target...)
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
