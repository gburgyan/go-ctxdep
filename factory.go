package ctxdep

import (
	"context"
	"fmt"
	"reflect"
)

// factoryWrapper wraps a factory function with metadata needed for validation and execution
type factoryWrapper struct {
	// The original function provided to Factory()
	originalFunc any
	// The type of the factory function that will be returned (e.g., UserFactory)
	targetType reflect.Type
	// Indices of parameters that come from context
	contextParamIndices []int
	// Indices of parameters that come from the factory function call
	factoryParamIndices []int
	// The return type info
	returnTypes []reflect.Type
	// Whether this factory returns an error as last value
	hasError bool
}

// Factory registers a function as a factory. The function must have at least one parameter
// that can be filled from the dependency context, and may have additional parameters that
// will be provided when the factory function is called.
//
// The type parameter T must be a function type that matches the signature of the factory
// after context dependencies are injected.
//
// Example:
//
//	type UserFactory func(ctx context.Context, userID string) (*User, error)
//
//	func UserLookup(ctx context.Context, db *Database, userID string) (*User, error) {
//	    // implementation
//	}
//
//	ctx := NewDependencyContext(ctx, db, Factory[UserFactory](UserLookup))
//	factory := Get[UserFactory](ctx)
//	user, err := factory(ctx, "user123")
func Factory[T any](fn any) any {
	targetType := reflect.TypeOf((*T)(nil)).Elem()
	if targetType.Kind() != reflect.Func {
		panic(fmt.Sprintf("Factory target type must be a function, got %v", targetType))
	}

	fnType := reflect.TypeOf(fn)
	if fnType.Kind() != reflect.Func {
		panic(fmt.Sprintf("Factory argument must be a function, got %v", fnType))
	}

	wrapper := &factoryWrapper{
		originalFunc: fn,
		targetType:   targetType,
	}

	// Analyze the function to determine which parameters come from context
	// and which come from the factory call
	wrapper.analyzeFunction()

	return wrapper
}

// analyzeFunction analyzes the original function to determine parameter sources
func (fw *factoryWrapper) analyzeFunction() {
	fnType := reflect.TypeOf(fw.originalFunc)
	targetType := fw.targetType

	// Collect return types
	for i := 0; i < fnType.NumOut(); i++ {
		outType := fnType.Out(i)
		fw.returnTypes = append(fw.returnTypes, outType)
		if outType == errorType {
			fw.hasError = true
		}
	}

	// Verify return types match
	if fnType.NumOut() != targetType.NumOut() {
		panic(fmt.Sprintf("Factory function return count mismatch: original has %d, target has %d",
			fnType.NumOut(), targetType.NumOut()))
	}
	for i := 0; i < fnType.NumOut(); i++ {
		if fnType.Out(i) != targetType.Out(i) {
			panic(fmt.Sprintf("Factory function return type mismatch at position %d: original has %v, target has %v",
				i, fnType.Out(i), targetType.Out(i)))
		}
	}

	// Verify target has context parameter
	hasTargetContext := false
	targetNonContextParams := 0
	for i := 0; i < targetType.NumIn(); i++ {
		if targetType.In(i) == contextType {
			hasTargetContext = true
		} else {
			targetNonContextParams++
		}
	}

	if !hasTargetContext {
		panic("Factory target type must have context.Context parameter")
	}

	// Count how many parameters we can't fill from context (these become factory params)
	// This is just a rough check - full validation happens during processFactories
	nonContextParams := 0
	for i := 0; i < fnType.NumIn(); i++ {
		if fnType.In(i) != contextType {
			nonContextParams++
		}
	}

	// We can't do full validation here without knowing what's in the context
	// But we can check if it's obviously wrong:
	// Original must have at least as many non-context params as target
	// (minus those that could come from dependencies)

	// Minimum case: all non-context params of original come from dependencies
	// except those matching target's non-context params

	// Maximum non-context params that could be dependencies
	maxDependencyParams := nonContextParams - targetNonContextParams

	// If we have negative dependency params, that's impossible
	if maxDependencyParams < 0 {
		panic(fmt.Sprintf("Factory parameter mismatch: target expects %d non-context parameters but original function can only provide %d at most",
			targetNonContextParams, nonContextParams))
	}
}

// validateAndCreateFactory validates that a factory can be created and returns the factory function
func (d *DependencyContext) validateAndCreateFactory(fw *factoryWrapper) (any, error) {
	fnType := reflect.TypeOf(fw.originalFunc)
	targetType := fw.targetType

	// Determine which parameters can be filled from context
	var contextParams []int
	var factoryParams []int
	var factoryParamTypes []reflect.Type

	for i := 0; i < fnType.NumIn(); i++ {
		paramType := fnType.In(i)

		// Check if this parameter can be filled from context
		if paramType == contextType {
			// context.Context is always available
			contextParams = append(contextParams, i)
		} else {
			// Try to create a pointer to this type and check if it's available
			paramPtr := reflect.New(paramType)
			if d.hasApplicableDependency(paramPtr.Interface()) {
				contextParams = append(contextParams, i)
			} else {
				factoryParams = append(factoryParams, i)
				factoryParamTypes = append(factoryParamTypes, paramType)
			}
		}
	}

	// Verify that we have at least one context parameter
	if len(contextParams) == 0 {
		return nil, fmt.Errorf("factory function must have at least one parameter that can be filled from context")
	}

	// Check if target requires context.Context parameter
	targetHasContext := false
	for i := 0; i < targetType.NumIn(); i++ {
		if targetType.In(i) == contextType {
			targetHasContext = true
			break
		}
	}

	// If target requires context, original must have it too
	if targetHasContext {
		hasOriginalContext := false
		for i := 0; i < fnType.NumIn(); i++ {
			if fnType.In(i) == contextType {
				hasOriginalContext = true
				break
			}
		}
		if !hasOriginalContext {
			return nil, fmt.Errorf("factory target type requires context.Context parameter but original function doesn't have one")
		}
	}

	// Count non-context parameters in target
	targetNonContextParams := 0
	hasTargetContext := false
	for i := 0; i < targetType.NumIn(); i++ {
		if targetType.In(i) == contextType {
			hasTargetContext = true
		} else {
			targetNonContextParams++
		}
	}

	// Verify target has context parameter
	if !hasTargetContext {
		return nil, fmt.Errorf("factory target type must have context.Context as first parameter")
	}

	// Verify factory parameter count matches
	if len(factoryParamTypes) != targetNonContextParams {
		return nil, fmt.Errorf("factory parameter count mismatch: original has %d non-dependency parameters, target expects %d",
			len(factoryParamTypes), targetNonContextParams)
	}

	// Verify factory parameter types match target function
	targetParamIndex := 0
	for i := 0; i < targetType.NumIn(); i++ {
		targetParamType := targetType.In(i)
		if targetParamType == contextType {
			// Skip context parameters in target
			continue
		}
		if targetParamIndex >= len(factoryParamTypes) {
			return nil, fmt.Errorf("factory function parameter count mismatch")
		}
		if targetParamType != factoryParamTypes[targetParamIndex] {
			return nil, fmt.Errorf("factory function parameter type mismatch at position %d: expected %v, got %v",
				i, targetParamType, factoryParamTypes[targetParamIndex])
		}
		targetParamIndex++
	}

	// Create the factory function
	factoryFunc := reflect.MakeFunc(targetType, func(args []reflect.Value) []reflect.Value {
		// Build the full parameter list for the original function
		fullParams := make([]reflect.Value, fnType.NumIn())

		// Fill context parameters
		var ctx context.Context
		factoryArgIndex := 0

		// Extract context from factory args if present
		for i := 0; i < len(args); i++ {
			if args[i].Type() == contextType {
				ctx = args[i].Interface().(context.Context)
				break
			}
		}

		if ctx == nil {
			panic("factory function requires context.Context parameter")
		}

		// Fill parameters from context using the dependency context from creation time
		for _, idx := range contextParams {
			paramType := fnType.In(idx)
			if paramType == contextType {
				fullParams[idx] = reflect.ValueOf(ctx)
			} else {
				paramPtr := reflect.New(paramType)
				err := d.FillDependency(ctx, paramPtr.Interface())
				if err != nil {
					// Return error if the function has error return
					if fw.hasError {
						errorResults := make([]reflect.Value, len(fw.returnTypes))
						for i, rt := range fw.returnTypes {
							if rt == errorType {
								errorResults[i] = reflect.ValueOf(err)
							} else {
								errorResults[i] = reflect.Zero(rt)
							}
						}
						return errorResults
					}
					panic(fmt.Sprintf("failed to resolve dependency for factory: %v", err))
				}
				fullParams[idx] = paramPtr.Elem()
			}
		}

		// Fill parameters from factory call
		for _, idx := range factoryParams {
			// Skip context parameters in args
			for factoryArgIndex < len(args) && args[factoryArgIndex].Type() == contextType {
				factoryArgIndex++
			}
			if factoryArgIndex >= len(args) {
				panic("not enough arguments provided to factory function")
			}
			fullParams[idx] = args[factoryArgIndex]
			factoryArgIndex++
		}

		// Call the original function
		fnValue := reflect.ValueOf(fw.originalFunc)
		return fnValue.Call(fullParams)
	})

	return factoryFunc.Interface(), nil
}

// processFactories validates and creates factory functions after regular dependencies are initialized
func (d *DependencyContext) processFactories() {
	var factories []*factoryWrapper

	// First pass: collect all factories
	d.slots.Range(func(key, value any) bool {
		s := value.(*slot)
		if fw, ok := s.value.(*factoryWrapper); ok {
			factories = append(factories, fw)
			// Remove the factory wrapper from slots temporarily
			d.slots.Delete(key)
		}
		return true
	})

	// Second pass: validate and create factory functions
	for _, fw := range factories {
		factoryFunc, err := d.validateAndCreateFactory(fw)
		if err != nil {
			panic(fmt.Sprintf("failed to create factory for type %v: %v", fw.targetType, err))
		}

		// Store the created factory function
		s := &slot{
			value:           factoryFunc,
			slotType:        fw.targetType,
			status:          StatusFactory,
			factoryOriginal: fw.originalFunc,
		}
		d.slots.Store(fw.targetType, s)
	}
}
