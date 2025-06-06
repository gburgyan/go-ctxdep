package ctxdep

import (
	"context"
	"fmt"
	"reflect"
)

// Validate creates a validation option that can be passed to NewDependencyContext.
// The validator function can take any number of parameters that will be resolved from
// the dependency context, and must return an error.
//
// Example:
//
//	func validateOrder(ctx context.Context, db *Database, order *Order) error {
//	    // validation logic
//	    return nil
//	}
//
//	ctx := NewDependencyContext(parent,
//	    db,
//	    order,
//	    Validate(validateOrder),
//	)
func Validate(validator any) any {
	vType := reflect.TypeOf(validator)
	if vType.Kind() != reflect.Func {
		panic(fmt.Sprintf("Validate argument must be a function, got %v", vType))
	}

	// Verify the function returns exactly one value of type error
	if vType.NumOut() != 1 {
		panic(fmt.Sprintf("validator must return exactly one value (error), got %d", vType.NumOut()))
	}
	if vType.Out(0) != errorType {
		panic(fmt.Sprintf("validator must return error, got %v", vType.Out(0)))
	}

	// At least one parameter is required
	if vType.NumIn() == 0 {
		panic("validator must have at least one parameter")
	}

	return &validatorWrapper{
		fn: validator,
	}
}

// validatorWrapper wraps a validator function
type validatorWrapper struct {
	fn any
}

// runValidators executes all registered validators in the context
func (d *DependencyContext) runValidators(ctx context.Context) error {
	// Run each validator
	for _, v := range d.validators {
		vw := v.(*validatorWrapper)
		if err := d.runValidator(ctx, vw); err != nil {
			return err
		}
	}

	return nil
}

// runValidator executes a single validator function
func (d *DependencyContext) runValidator(ctx context.Context, vw *validatorWrapper) error {
	fnType := reflect.TypeOf(vw.fn)
	fnValue := reflect.ValueOf(vw.fn)

	// Build parameter list by resolving dependencies
	params := make([]reflect.Value, fnType.NumIn())
	for i := 0; i < fnType.NumIn(); i++ {
		paramType := fnType.In(i)

		if paramType == contextType {
			params[i] = reflect.ValueOf(ctx)
		} else {
			// Create a pointer to the parameter type
			paramPtr := reflect.New(paramType)

			// Try to fill the dependency
			err := d.FillDependency(ctx, paramPtr.Interface())
			if err != nil {
				return fmt.Errorf("validator dependency resolution failed for type %v: %w", paramType, err)
			}

			params[i] = paramPtr.Elem()
		}
	}

	// Call the validator
	results := fnValue.Call(params)

	// Extract the error result
	if !results[0].IsNil() {
		return results[0].Interface().(error)
	}

	return nil
}
