package ctxdep

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"sync"
)

type key int

const dependencyContextKey key = 0

// DependencyContext is the mediator of the dependency injection framework in this library.
// It maintains a collection of dependencies that were introduced to the context as well
// as generators that can be used to lazily create the dependencies as they are needed.
//
// Dependencies come in two forms:
//   - Directly injected dependencies
//   - Generator-based dependencies
//
// A directly injected dependency is simply an object that is inserted into the context
// and can be searched for by type.
//
// A generator-based dependency is a function that is added to the context that can be
// used to create one or more dependencies that will subsequently be stored in the context.
// A generator is a function in the form: func (context.Context) (returnA, returnB, error)
//
//   - The generator must have a single parameter, context.Context.
//   - The generator must return at least one non-error object
//   - The generator may return one error
//
// A generator is called if one of the return types is requested such that the value is
// not yet known. If a generator is called that returns multiple values, all the returned
// values will be stored in the context for ske of efficiency.
//
// Generators may also be wrapped with Immediate() which will cause the generators passed
// to it to be called immediately in a different goroutine.
//
// If a type is requested that is not directly found as either a directly injected
// or generator-based dependency, then a search if conducted to see if any dependency may
// be cast to the requested type. If a suitable dependency is found, that will be returned.
// This can be the case if a dependency is a struct that implements an interface--if an
// interface is requested, it will not be found in the initial lookup. The subsequent search
// will find that the struct can be used as the interface and that will be returned.
type DependencyContext struct {
	// parentContext holds the base context this was built on top of. This allows to have
	// several layered DependencyContext objects on the context stack that have differing
	// lifetimes.
	parentContext context.Context

	// slots is a map keyed on the Type of the object that is either held by, or can be
	// generated by a generator.
	slots map[reflect.Type]*slot
}

// slot stored the internal state of a dependency slot.
type slot struct {
	value     interface{}
	generator interface{}
	slotType  reflect.Type
	lock      sync.Mutex
	immediate bool
}

var errorType = reflect.TypeOf((*error)(nil)).Elem()
var contextType = reflect.TypeOf((*context.Context)(nil)).Elem()

// AddDependencies adds the given dependencies to the context. This will add all the deps
// passed in and treat them as a generator if it's a function or a direct dependency
// if it's not.
func (d *DependencyContext) AddDependencies(ctx context.Context, deps ...interface{}) {
	d.addDependencies(deps, false)
	d.resolveImmediateDependencies(ctx)
}

func (d *DependencyContext) addDependencies(deps []interface{}, immediate bool) {
	for _, dep := range deps {
		if immediateDependencies, ok := dep.(*immediateDependencies); ok {
			d.addDependencies(immediateDependencies.dependencies, true)
		} else {
			depType := reflect.TypeOf(dep)
			depKind := depType.Kind()
			if depKind == reflect.Func {
				d.addGenerator(dep, immediate)
			} else if depKind == reflect.Pointer {
				d.addValue(depType, dep)
			}
		}
	}
}

// addValue adds a direct dependency to the dependency context.
func (d *DependencyContext) addValue(depType reflect.Type, dep interface{}) {
	kind := depType.Kind()
	if (kind == reflect.Pointer || kind == reflect.Interface) && dep == nil {
		panic(fmt.Sprintf("invalid nil value dependency for type %v", depType))
	}
	// A value may override an existing slot.
	s := &slot{
		value:    dep,
		slotType: depType,
	}
	d.slots[depType] = s
}

func (d *DependencyContext) GetWithError(ctx context.Context, targets ...interface{}) error {
	for _, target := range targets {
		err := d.getDependency(ctx, target)
		if err != nil {
			return err
		}
	}
	log.Default()

	return nil
}

func (d *DependencyContext) Get(ctx context.Context, target ...interface{}) {
	err := d.GetWithError(ctx, target...)
	if err != nil {
		panic(err)
	}
}

func (d *DependencyContext) getDependency(ctx context.Context, target interface{}) error {
	s, err := d.findApplicableSlot(target)
	if err != nil {
		pdc := d.parentDependencyContext()
		if pdc != nil {
			return pdc.GetWithError(ctx, target)
		}
		return err
	}

	err = d.getValue(ctx, s, target)
	if err != nil {
		return err
	}
	return nil
}

// findApplicableSlot looks for an appropriate slot that can fulfil the requested target. If
// the slot is directly found by the request type, simply return it. Otherwise, look for another
// slot that can be assigned to the target and return that if fount. Returns nil if
// nothing is suitable.
func (d *DependencyContext) findApplicableSlot(target interface{}) (*slot, error) {
	pt := reflect.TypeOf(target)
	if pt.Kind() != reflect.Pointer {
		panic(fmt.Sprintf("target must be a pointer type: %v", pt))
	}
	requestType := pt.Elem()

	if s, ok := d.slots[requestType]; ok {
		return s, nil
	}

	for slotTarget, s := range d.slots {
		if requestType.Kind() == reflect.Interface && slotTarget.AssignableTo(requestType) {
			return s, nil
		}
	}

	return nil, &DependencyError{
		Message:        "slot not found for requested type",
		ReferencedType: requestType,
		Status:         d.Status(),
	}
}

// getValue fills in the target value from this slot. If the value is already there through
// either a direct dependency or if it was previously generated then simply return then. Otherwise,
// either use the generator to make a value or delegate to an upstream DependencyContext.
// The precondition for the function is that the slot's type matches the target such that the
// slot can be assigned to target.
func (d *DependencyContext) getValue(ctx context.Context, activeSlot *slot, target interface{}) error {
	// Before locking this slot, ensure that we're not in a cyclic dependency. If we are,
	// return an error. Otherwise, the lock call would deadlock.
	cycleCtx, unlocker, err := enterSlotProcessing(ctx, activeSlot)
	if err != nil {
		return err
	}
	defer unlocker()

	targetVal := reflect.ValueOf(target)
	targetType := reflect.TypeOf(target).Elem()

	// If we have a value in this slot, then we can simply return it without any locking. This
	// has a slight race condition where a slot's value is added after this check but before
	// the lock. The same test after the lock makes the race not important.
	//
	// This is here as an optimization to prevent the code from acquiring the locks if we
	// don't need to.
	if activeSlot.value != nil {
		slotVal := reflect.ValueOf(activeSlot.value)
		targetVal.Elem().Set(slotVal)
		return nil
	}

	// Preemptively lock all the potential outputs from a generator for this slot, if it exists. We
	// need to ensure that the locks are acquired in the same order in all cases to prevent potential
	// deadlocks.
	resultSlots := d.getGeneratorOutputSlots(activeSlot)
	for _, resultSlot := range resultSlots {
		resultSlot.lock.Lock()
		//goland:noinspection GoDeferInLoop
		defer resultSlot.lock.Unlock()
	}

	// This is the same check as above, but now completely thread safe.
	if activeSlot.value != nil {
		slotVal := reflect.ValueOf(activeSlot.value)
		targetVal.Elem().Set(slotVal)
		return nil
	}

	// A slot either has a value or a generator. We don't have a value, so call the generator.
	results, err := d.invokeSlotGenerator(cycleCtx, activeSlot)
	if err != nil {
		return err
	}

	// Check if there's an error that was returned.
	err = d.getGeneratorError(results)
	if err != nil {
		return &DependencyError{
			Message:        "error running generator",
			ReferencedType: activeSlot.slotType,
			Status:         d.Status(),
			SourceError:    err,
		}
	}

	// No errors, so gather the results and fill that value in to the dependency context.
	err = d.mapGeneratorResults(results, targetType, targetVal)
	if err != nil {
		return &DependencyError{
			Message:        "error mapping generator results to context",
			ReferencedType: activeSlot.slotType,
			Status:         d.Status(),
			SourceError:    err,
		}
	}

	return nil
}

// parentDependencyContext returns the next DependencyContext up the context stack if it
// exists. Otherwise, this returns nil.
func (d *DependencyContext) parentDependencyContext() *DependencyContext {
	pdcAny := d.parentContext.Value(dependencyContextKey)
	if pdcAny == nil {
		return nil
	}
	if pdc, ok := pdcAny.(*DependencyContext); ok {
		return pdc
	}
	// There should be no normal way to get to this point.
	panic("unexpected context value of parent dependency context")
}
