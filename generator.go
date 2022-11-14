package ctxdep

import (
	"context"
	"reflect"
)

// addGenerator validates the generator function and adds it to the dependency context
// assuming it's valid. If it's not valid this function panics.
func (d *DependencyContext) addGenerator(generatorFunction interface{}, immediate bool) {
	funcType := reflect.TypeOf(generatorFunction)

	if funcType.Kind() != reflect.Func {
		// double-checking this because it's cheap. There should be no
		// public way to get here.
		panic("generator must be a function")
	}

	hasError := false
	var resultTypes []reflect.Type

	for i := 0; i < funcType.NumOut(); i++ {
		resultType := funcType.Out(i)
		if resultType.AssignableTo(errorType) {
			if hasError {
				panic("multiple error results on a generator function not permitted")
			}
			hasError = true
		} else {
			resultTypes = append(resultTypes, resultType)
		}
	}

	if len(resultTypes) == 0 {
		panic("generator must have at least one result value")
	}

	for _, resultType := range resultTypes {
		s := &slot{
			generator: generatorFunction,
			slotType:  resultType,
			immediate: immediate,
		}
		d.slots[resultType] = s
	}
}

// getGeneratorError finds the error result from a generator, if it exists. If no error is present
// or it doesn't have an error, this returns nil.
func (d *DependencyContext) getGeneratorError(results []reflect.Value) error {
	for _, result := range results {
		if result.CanConvert(errorType) {
			return result.Convert(errorType).Interface().(error)
		}
	}
	return nil
}

// invokeSlotGenerator calls the slot's generator function and returns the results of the call.
func (d *DependencyContext) invokeSlotGenerator(ctx context.Context, activeSlot *slot) ([]reflect.Value, error) {
	genType := reflect.TypeOf(activeSlot.generator)
	inCount := genType.NumIn()
	params := make([]reflect.Value, inCount)
	for i := 0; i < inCount; i++ {
		inType := genType.In(i)
		if inType == contextType {
			params[i] = reflect.ValueOf(ctx)
		} else {
			paramPointerValue := reflect.New(inType)
			targetTypePointer := paramPointerValue.Interface()
			err := d.getDependency(ctx, targetTypePointer)
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
func (d *DependencyContext) mapGeneratorResults(results []reflect.Value, targetType reflect.Type, targetVal reflect.Value) {
	for _, result := range results {
		resultType := result.Type()
		if resultType.AssignableTo(errorType) {
			// already handled
			continue
		}
		if resultType.ConvertibleTo(targetType) {
			// This is the type that was asked for so fill in the target
			// with the value. We'll set slots later on.
			targetVal.Elem().Set(result)
		}

		if resultSlot, ok := d.slots[resultType]; ok {
			if resultSlot.value == nil {
				resultSlot.value = result.Interface()
			}
		} else {
			// We should never get this since the addGenerator call
			// should have pre-created these.
			d.slots[resultType] = &slot{value: result.Interface()}
		}

		if targetType != resultType {
			// Now handle the case where we asked for something like an
			// interface and found this slot. This fills in the lookup
			// directly for the actual requested type so if it's
			// requested again we can find it directly.
			if targetSlot, ok := d.slots[targetType]; ok {
				if targetSlot.value == nil {
					targetSlot.value = result.Interface()
				}
			} else {
				d.slots[resultType] = &slot{value: result.Interface()}
			}
		}
	}
}

// getGeneratorOutputSlots gets the slots for the generator of this slot. When a generator
// is run, we need to ensure that we lock the slots in the same order to prevent deadlocks.
func (d *DependencyContext) getGeneratorOutputSlots(activeSlot *slot) []*slot {
	if activeSlot.generator == nil {
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
		result = append(result, d.slots[retType])
	}
	return result
}
