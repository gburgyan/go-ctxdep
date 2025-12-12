package ctxdep

import (
	"fmt"
	"reflect"
)

// optionalWrapper is an internal wrapper to signal that a nil pointer
// should be silently skipped instead of panicking.
type optionalWrapper struct {
	dependency any
}

// Optional wraps a dependency to allow typed nil pointers to be silently skipped.
// If the dependency is nil, it is not added to the context.
// If the dependency is non-nil, it is added as a normal direct dependency.
//
// Constraints:
//   - Only pointer and interface types are allowed
//   - Generators (functions) cannot be optional
//   - Cannot be combined with Immediate() or Overrideable()
//
// Primary use case is testing where dependencies may be conditionally provided:
//
//	var mockDB *MockDatabase // may be nil in some tests
//	ctx := NewDependencyContext(ctx, Optional(mockDB), ...)
//	// If mockDB is nil, no dependency is added
//	// If mockDB is non-nil, it's added as a normal dependency
func Optional(dep any) *optionalWrapper {
	return &optionalWrapper{
		dependency: dep,
	}
}

// processOptionalDependency handles optional dependencies.
// If nil, silently skipped. If non-nil, added as normal dependency.
func (d *DependencyContext) processOptionalDependency(ow *optionalWrapper) {
	dep := ow.dependency

	// Validate: cannot wrap other wrappers
	if _, ok := dep.(*immediateDependencies); ok {
		panic("Optional() cannot wrap Immediate()")
	}
	if _, ok := dep.(*overrideableWrapper); ok {
		panic("Optional() cannot wrap Overrideable()")
	}
	if _, ok := dep.(*optionalWrapper); ok {
		panic("Optional() cannot wrap another Optional()")
	}
	if _, ok := dep.(*adaptWrapper); ok {
		panic("Optional() cannot wrap Adapt()")
	}
	if _, ok := dep.(*validatorWrapper); ok {
		panic("Optional() cannot wrap Validate()")
	}

	depType := reflect.TypeOf(dep)
	if depType == nil {
		return // untyped nil - skip silently
	}

	// Validate: cannot be a function (generator)
	if depType.Kind() == reflect.Func {
		panic("Optional() cannot wrap a generator function")
	}

	// Validate: must be pointer or interface type
	kind := depType.Kind()
	if kind != reflect.Pointer && kind != reflect.Interface {
		panic(fmt.Sprintf("Optional() requires a pointer or interface type, got: %s", depType.String()))
	}

	// Check if nil
	if reflect.ValueOf(dep).IsNil() {
		return // typed nil - skip silently
	}

	// Non-nil value - add as normal dependency
	d.addValue(depType, dep)
}
