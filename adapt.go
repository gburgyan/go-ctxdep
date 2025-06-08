package ctxdep

import (
	"context"
	"fmt"
	"reflect"
)

// adaptWrapper wraps an adapted function with metadata needed for validation and execution
type adaptWrapper struct {
	// The original function provided to Adapt()
	originalFunc any
	// The type of the adapted function that will be returned (e.g., UserAdapter)
	targetType reflect.Type
	// Indices of parameters that come from context
	contextParamIndices []int
	// Indices of parameters that come from the adapted function call
	adaptParamIndices []int
	// The return type info
	returnTypes []reflect.Type
	// Whether this adapted function returns an error as last value
	hasError bool
}

// Adapt registers a function as an adapter. The function must have at least one parameter
// that can be filled from the dependency context, and may have additional parameters that
// will be provided when the adapted function is called.
//
// The type parameter T must be a function type that matches the signature of the adapted function
// after context dependencies are injected.
//
// Example:
//
//	type UserAdapter func(ctx context.Context, userID string) (*User, error)
//
//	func UserLookup(ctx context.Context, db *Database, userID string) (*User, error) {
//	    // implementation
//	}
//
//	ctx := NewDependencyContext(ctx, db, Adapt[UserAdapter](UserLookup))
//	adapter := Get[UserAdapter](ctx)
//	user, err := adapter(ctx, "user123")
func Adapt[T any](fn any) any {
	targetType := reflect.TypeOf((*T)(nil)).Elem()
	if targetType.Kind() != reflect.Func {
		panic(fmt.Sprintf("Adapt target type must be a function, got %v", targetType))
	}

	fnType := reflect.TypeOf(fn)
	if fnType.Kind() != reflect.Func {
		panic(fmt.Sprintf("Adapt argument must be a function, got %v", fnType))
	}

	wrapper := &adaptWrapper{
		originalFunc: fn,
		targetType:   targetType,
	}

	// Analyze the function to determine which parameters come from context
	// and which come from the adapter call
	wrapper.analyzeFunction()

	return wrapper
}

// analyzeFunction analyzes the original function to determine parameter sources
func (aw *adaptWrapper) analyzeFunction() {
	fnType := reflect.TypeOf(aw.originalFunc)
	targetType := aw.targetType

	// Collect return types
	for i := 0; i < fnType.NumOut(); i++ {
		outType := fnType.Out(i)
		aw.returnTypes = append(aw.returnTypes, outType)
		if outType == errorType {
			aw.hasError = true
		}
	}

	// Verify return types match
	if fnType.NumOut() != targetType.NumOut() {
		panic(fmt.Sprintf("Adapted function return count mismatch: original has %d, target has %d",
			fnType.NumOut(), targetType.NumOut()))
	}
	for i := 0; i < fnType.NumOut(); i++ {
		if fnType.Out(i) != targetType.Out(i) {
			panic(fmt.Sprintf("Adapted function return type mismatch at position %d: original has %v, target has %v",
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
		panic("Adapter target type must have context.Context parameter")
	}

	// Count how many parameters we can't fill from context (these become adapter params)
	// This is just a rough check - full validation happens during processAdapters
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
		panic(fmt.Sprintf("Adapter parameter mismatch: target expects %d non-context parameters but original function can only provide %d at most",
			targetNonContextParams, nonContextParams))
	}
}

// validateAndCreateAdapter validates that an adapter can be created and returns the adapted function
func (d *DependencyContext) validateAndCreateAdapter(aw *adaptWrapper) (any, error) {
	fnType := reflect.TypeOf(aw.originalFunc)
	targetType := aw.targetType

	// First, identify which target parameters we need to match (excluding context)
	var targetParamTypes []reflect.Type
	for i := 0; i < targetType.NumIn(); i++ {
		if targetType.In(i) != contextType {
			targetParamTypes = append(targetParamTypes, targetType.In(i))
		}
	}

	// Now categorize original function parameters
	var contextParams []int
	var adaptParams []int
	var adaptParamTypes []reflect.Type

	for i := 0; i < fnType.NumIn(); i++ {
		paramType := fnType.In(i)

		// Check if this parameter can be filled from context
		if paramType == contextType {
			// context.Context is always available
			contextParams = append(contextParams, i)
		} else {
			// Check if this parameter type matches any target parameter type
			matchesTarget := false
			for _, targetParamType := range targetParamTypes {
				if paramType == targetParamType {
					matchesTarget = true
					break
				}
			}

			// If it matches a target param type AND is available in context,
			// we need to decide whether to treat it as runtime or context param
			paramPtr := reflect.New(paramType)
			availableInContext := d.hasApplicableDependency(paramPtr.Interface())

			if matchesTarget && availableInContext && len(adaptParams) < len(targetParamTypes) {
				// This parameter matches a target type and we still need runtime params
				// Treat it as a runtime parameter
				adaptParams = append(adaptParams, i)
				adaptParamTypes = append(adaptParamTypes, paramType)
			} else if availableInContext {
				// Available in context and either doesn't match target or we have enough runtime params
				contextParams = append(contextParams, i)
			} else {
				// Not available in context, must be a runtime parameter
				adaptParams = append(adaptParams, i)
				adaptParamTypes = append(adaptParamTypes, paramType)
			}
		}
	}

	// Verify that we have at least one context parameter
	if len(contextParams) == 0 {
		return nil, fmt.Errorf("adapted function must have at least one parameter that can be filled from context")
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
			return nil, fmt.Errorf("adapter target type requires context.Context parameter but original function doesn't have one")
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

	// For adapters with no runtime parameters, we don't require context in target
	// This allows for simpler function signatures when all params come from dependencies
	if targetNonContextParams == 0 && !hasTargetContext {
		// This is OK - it's a no-parameter adapter function
	} else if targetNonContextParams > 0 && !hasTargetContext {
		return nil, fmt.Errorf("adapter target type must have context.Context parameter when it has other parameters")
	}

	// Special case: if all parameters can be filled from context but target expects parameters,
	// we need to check if some of those context parameters should actually be runtime parameters
	if len(adaptParamTypes) == 0 && targetNonContextParams > 0 {
		// This could be valid if we're intentionally exposing some context parameters as runtime parameters
		// We'll allow this and handle it in the adapted function
	} else if len(adaptParamTypes) != targetNonContextParams {
		return nil, fmt.Errorf("adapter parameter count mismatch: original has %d non-dependency parameters, target expects %d",
			len(adaptParamTypes), targetNonContextParams)
	}

	// Verify adapter parameter types match target function
	targetParamIndex := 0
	for i := 0; i < targetType.NumIn(); i++ {
		targetParamType := targetType.In(i)
		if targetParamType == contextType {
			// Skip context parameters in target
			continue
		}
		if targetParamIndex >= len(adaptParamTypes) {
			return nil, fmt.Errorf("adapted function parameter count mismatch")
		}
		if targetParamType != adaptParamTypes[targetParamIndex] {
			return nil, fmt.Errorf("adapted function parameter type mismatch at position %d: expected %v, got %v",
				i, targetParamType, adaptParamTypes[targetParamIndex])
		}
		targetParamIndex++
	}

	// Create the adapted function
	adaptedFunc := reflect.MakeFunc(targetType, func(args []reflect.Value) []reflect.Value {
		// Build the full parameter list for the original function
		fullParams := make([]reflect.Value, fnType.NumIn())

		// Fill context parameters
		var ctx context.Context
		adaptArgIndex := 0

		// Extract context from adapter args if present
		for i := 0; i < len(args); i++ {
			if args[i].Type() == contextType {
				ctx = args[i].Interface().(context.Context)
				break
			}
		}

		if ctx == nil {
			panic("adapted function requires context.Context parameter")
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
					if aw.hasError {
						errorResults := make([]reflect.Value, len(aw.returnTypes))
						for i, rt := range aw.returnTypes {
							if rt == errorType {
								errorResults[i] = reflect.ValueOf(err)
							} else {
								errorResults[i] = reflect.Zero(rt)
							}
						}
						return errorResults
					}
					panic(fmt.Sprintf("failed to resolve dependency for adapter: %v", err))
				}
				fullParams[idx] = paramPtr.Elem()
			}
		}

		// Fill parameters from adapted call
		for _, idx := range adaptParams {
			// Skip context parameters in args
			for adaptArgIndex < len(args) && args[adaptArgIndex].Type() == contextType {
				adaptArgIndex++
			}
			if adaptArgIndex >= len(args) {
				panic("not enough arguments provided to adapted function")
			}
			fullParams[idx] = args[adaptArgIndex]
			adaptArgIndex++
		}

		// Call the original function
		fnValue := reflect.ValueOf(aw.originalFunc)
		return fnValue.Call(fullParams)
	})

	return adaptedFunc.Interface(), nil
}

// processAdapters validates and creates adapted functions after regular dependencies are initialized
func (d *DependencyContext) processAdapters() {
	var adapters []*adaptWrapper

	// First pass: collect all adapters
	d.slots.Range(func(key, value any) bool {
		s := value.(*slot)
		if aw, ok := s.value.(*adaptWrapper); ok {
			adapters = append(adapters, aw)
			// Remove the adapter wrapper from slots temporarily
			d.slots.Delete(key)
		}
		return true
	})

	// Second pass: validate and create adapted functions
	for _, aw := range adapters {
		adaptedFunc, err := d.validateAndCreateAdapter(aw)
		if err != nil {
			panic(fmt.Sprintf("failed to create adapter for type %v: %v", aw.targetType, err))
		}

		// Store the created adapted function
		s := &slot{
			value:           adaptedFunc,
			slotType:        aw.targetType,
			status:          StatusAdapter,
			adapterOriginal: aw.originalFunc,
		}
		d.slots.Store(aw.targetType, s)
	}
}
