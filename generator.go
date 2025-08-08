package ctxdep

import (
	"context"
	"fmt"
	"reflect"
)

// addGenerator validates the generator function and adds it to the dependency context
// assuming it's valid. If it's not valid this function panics.
func (d *DependencyContext) addGenerator(generatorFunction any, immediate *immediateDependencies) {
	funcType := reflect.TypeOf(generatorFunction)

	if funcType.Kind() != reflect.Func {
		// double-checking this because it's cheap. There should be no
		// public way to get here.
		panic("generator must be a function")
	}

	// Use cached type info for better performance
	typeInfo := getTypeInfo(funcType)

	if len(typeInfo.funcReturns) == 0 {
		panic("generator must have at least one result value")
	}

	for _, resultType := range typeInfo.funcReturns {
		if existingSlotA, existing := d.slots.Load(resultType); existing {
			existingSlot := existingSlotA.(*slot)
			if !d.loose && !d.isOverrideable(resultType) {
				panic(fmt.Sprintf("generator result type %v already exists--a generator may not override an existing slot", resultType))
			}
			if existingSlot.value.Load() != nil {
				// Never override a concrete value
				return
			}
		}

		// Check if parent has this slot
		if !d.loose && !d.isOverrideable(resultType) {
			parent := d.parentDependencyContext()
			for parent != nil {
				if _, exists := parent.slots.Load(resultType); exists {
					if parent.locked {
						panic(fmt.Sprintf("cannot override dependency of type %v from locked parent context", resultType))
					} else {
						panic(fmt.Sprintf("generator result type %v already exists--a generator may not override an existing slot", resultType))
					}
				}
				parent = parent.parentDependencyContext()
			}
		}

		s := &slot{
			generator: generatorFunction,
			slotType:  resultType,
			immediate: immediate,
			status:    StatusGenerator,
		}
		d.slots.Store(resultType, s)
	}
}

// getGeneratorError finds the error result from a generator, if it exists. If no error is present,
// or it doesn't have an error, this returns nil.
func (d *DependencyContext) getGeneratorError(results []reflect.Value) error {
	for _, result := range results {
		typeInfo := getTypeInfo(result.Type())
		if typeInfo.assignableToError && !result.IsNil() {
			return result.Convert(errorType).Interface().(error)
		}
	}
	return nil
}

// invokeSlotGenerator calls the slot's generator function and returns the results of the call.
func (d *DependencyContext) invokeSlotGenerator(ctx context.Context, activeSlot *slot) ([]reflect.Value, error) {
	var sc context.Context
	if prevSc, ok := ctx.(*secureContext); ok {
		// We don't need to keep wrapping contexts if they are already wrapped.
		// This saves making the context chain too long in degenerate cases.
		sc = prevSc
	} else {
		sc = &secureContext{
			baseContext:   d.selfContext,
			timingContext: ctx,
		}
	}

	genType := reflect.TypeOf(activeSlot.generator)
	typeInfo := getTypeInfo(genType)
	params := make([]reflect.Value, len(typeInfo.funcParams))

	for i, inType := range typeInfo.funcParams {
		if inType == contextType {
			params[i] = reflect.ValueOf(sc)
		} else {
			paramPointerValue := reflect.New(inType)
			targetTypePointer := paramPointerValue.Interface()
			err := d.FillDependency(sc, targetTypePointer)
			if err != nil {
				return nil, err
			}
			params[i] = paramPointerValue.Elem()
		}
	}

	gv := reflect.ValueOf(activeSlot.generator)
	results := gv.Call(params)
	return results, nil
}

// mapGeneratorResults takes the results returned from the generator and fills in the various slots' values
// from the results.
func (d *DependencyContext) mapGeneratorResults(results []reflect.Value, targetType reflect.Type, targetVal reflect.Value) error {
	for _, result := range results {
		resultType := result.Type()
		if resultType.AssignableTo(errorType) {
			// already handled
			continue
		}
		if result.Kind() == reflect.Pointer && result.IsNil() {
			return &DependencyError{
				Message:        "generator returned nil result",
				ReferencedType: resultType,
				Status:         d.Status(),
			}
		}
		if resultType.ConvertibleTo(targetType) {
			// This is the type that was asked for so fill in the target
			// with the value. We'll set slots later on.
			targetVal.Elem().Set(result)
		}

		// Now save the result value to the slot for later use.
		if resultSlotA, ok := d.slots.Load(resultType); ok {
			resultSlot := resultSlotA.(*slot)
			if resultSlot.value.Load() == nil {
				val := result.Interface()
				resultSlot.value.Store(&val)
				resultSlot.status = StatusGenerator
			}
		} else {
			// We should never get this since the addGenerator call
			// should have pre-created these.
			s := &slot{status: StatusGenerator}
			val := result.Interface()
			s.value.Store(&val)
			d.slots.Store(resultType, s)
		}
	}
	return nil
}

// getGeneratorOutputSlots gets the slots for the generator of this slot. When a generator
// is run, we need to ensure that we lock the slots in the same order to prevent deadlocks.
func (d *DependencyContext) getGeneratorOutputSlots(activeSlot *slot) []*slot {
	if activeSlot.generator == nil {
		// There should be no way to get to this point.
		return nil
	}
	generatorType := reflect.TypeOf(activeSlot.generator)
	var result []*slot
	retVals := generatorType.NumOut()
	for i := 0; i < retVals; i++ {
		retType := generatorType.Out(i)
		if retType.AssignableTo(errorType) {
			// Errors are not a type of slot.
			continue
		}
		sa, ok := d.slots.Load(retType)
		if !ok {
			// This should be impossible.
			panic("generator output slot not found")
		}
		result = append(result, sa.(*slot))
	}
	return result
}

// isSlotValid verifies that the generator's dependencies can nominally be
// fulfilled by the dependencies present. This does not check for cyclic dependencies as
// that would be more expensive.
func (d *DependencyContext) isSlotValid(s *slot) bool {
	if s.value.Load() != nil {
		return true
	}
	genType := reflect.TypeOf(s.generator)
	if genType.Kind() != reflect.Func {
		// There should be no way of getting here.
		return false
	}
	inCount := genType.NumIn()
	for i := 0; i < inCount; i++ {
		inType := genType.In(i)
		if inType == contextType {
			continue
		} else {
			paramPointerValue := reflect.New(inType)
			targetTypePointer := paramPointerValue.Interface()
			hasDependency := d.hasApplicableDependency(targetTypePointer)
			if !hasDependency {
				return false
			}
		}
	}
	return true
}
