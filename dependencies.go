package ctxdep

import (
	"context"
	"fmt"
	"github.com/gburgyan/go-timing"
	"reflect"
	"sync"
)

type key int

const dependencyContextKey key = 0

// DependencyContext is the mediator of the dependency tracking framework in this library.
// It maintains a collection of dependencies that were introduced to the context as well
// as generators that can be used to lazily create the dependencies as they are needed.
//
// Dependencies come in two forms:
//   - Directly dependencies
//   - Generator-based dependencies
//
// A direct dependency is simply an object that is inserted into the context
// and can be searched for by type.
//
// A generator-based dependency is a function that is added to the context that can be
// used to create one or more dependencies that will subsequently be stored in the context.
// A generator is a function in the form: func (context.Context, *param1, *param2) (returnA, returnB, error)
//
//   - The generator may have a parameter, context.Context
//   - The generator may have any number of parameters that are pointers to types that are
//     already in the context, or can be generated by a generator in the context.
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
// If a type is requested that is not directly found as either a direct dependency
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

	// selfContext is the context that contains this DependencyContext.
	selfContext context.Context

	// slots is a map keyed on the Type of the object that is either held by, or can be
	// generated by a generator.
	slots sync.Map

	// loose controls if slots can be overridden during the construction of the DependencyContext.
	// The general use case of this would be for unit tests where there may be a general
	// set of dependencies that are added, but certain ones are overridden for use by the test
	// that is running. The default value of `false` will cause `panic`s if there are multiple
	// dependencies for the same type.
	loose bool

	// parentFixed controls if we are in a position to override the parent context. This
	// is only usable by the first added dependency.
	parentFixed bool
}

// slot stored the internal state of a dependency slot.
type slot struct {
	value     any
	generator any
	slotType  reflect.Type
	lock      sync.Mutex
	immediate *immediateDependencies
	status    SlotStatus
}

type SlotStatus int

const (
	StatusDirect     SlotStatus = iota // directly set dependency
	StatusGenerator                    // a generator ran to create this dependency
	StatusFromParent                   // imported from a parent dependency context (optimization)
)

var errorType = reflect.TypeOf((*error)(nil)).Elem()
var contextType = reflect.TypeOf((*context.Context)(nil)).Elem()

// addDependenciesAndInitialize adds the given dependencies to the context. This will add
// all the dependencies passed in and treat them as a generator if it's a function or
// a direct dependency if it's not. Validation is done to ensure that any generators that
// have been added to the context have parameters that can be resolved by the context. If
// there are unresolved dependencies, this will panic.
//
// After adding the dependencies to the context, any immediate dependencies will be resolved.
func (d *DependencyContext) addDependenciesAndInitialize(ctx context.Context, deps ...any) {
	d.addDependencies(deps, nil)
	d.validateDependencies()
	d.resolveImmediateDependencies(ctx)
}

// validateDependencies ensures that everything that was added is in a consistent state. If
// any dependencies exist that can't be fulfilled, this will `panic`.
func (d *DependencyContext) validateDependencies() {
	d.slots.Range(func(_, sa any) bool {
		s := sa.(*slot)
		if !d.isSlotValid(s) {
			panic(fmt.Sprintf("generator for %s has dependencies that cannot be resolved", formatGeneratorDebug(s.generator)))
		}
		return true
	})
}

// addDependencies adds the given dependencies to the context. This will add all the deps
// passed in and treat them as a generator if it's a function or a direct dependency
// if it's not. If a slice any  is passed in, then the contents of the slice are evaluate as
// if they were passed in directly. Validation is done to ensure that any generators that
// have been added to the context have parameters that can be resolved by the context. If
// there are unresolved dependencies, this will panic.
func (d *DependencyContext) addDependencies(deps []any, immediate *immediateDependencies) {
	for _, dep := range deps {
		// If this is the first dependency added we can override the parent context.
		if ctx, ok := dep.(context.Context); ok {
			if d.parentFixed {
				panic("cannot override parent context")
			}
			d.parentContext = ctx
			d.parentFixed = true
			continue
		}
		if immediateWrapper, ok := dep.(*immediateDependencies); ok {
			d.parentFixed = true
			d.addDependencies(immediateWrapper.dependencies, immediateWrapper)
		} else if subSlice, ok := dep.([]any); ok {
			d.addDependencies(subSlice, immediate)
			d.parentFixed = true
		} else {
			d.parentFixed = true
			depType := reflect.TypeOf(dep)
			if depType == nil {
				// This is a nil value, so we can't do anything with it.
				continue
			}
			depKind := depType.Kind()

			switch depKind {
			case reflect.Func:
				d.addGenerator(dep, immediate)

			case reflect.Pointer:
				d.addValue(depType, dep)

			default:
				panic(fmt.Sprintf("invalid dependency: %s", depType.String()))
			}
		}
	}
}

// addValue adds a direct dependency to the dependency context.
func (d *DependencyContext) addValue(depType reflect.Type, dep any) {
	kind := depType.Kind()
	if (kind == reflect.Pointer || kind == reflect.Interface) && reflect.ValueOf(dep).IsNil() {
		panic(fmt.Sprintf("invalid nil value dependency for type %v", depType))
	}
	if _, existing := d.slots.Load(depType); existing && !d.loose {
		panic(fmt.Sprintf("a slot for type %v already exists--value may not override an existing slot", depType))
	}
	// A value may override an existing slot.
	s := &slot{
		value:    dep,
		slotType: depType,
		status:   StatusDirect,
	}
	d.slots.Store(depType, s)
}

// GetBatch behaves like GetBatchWithError except it will panic if the requested dependencies are not
// found. The typical behavior for a dependency that is not found is returning an error or
// panicking on the caller's side, so this presents a simplified interface for getting the
// required dependencies.
func (d *DependencyContext) GetBatch(ctx context.Context, target ...any) {
	err := d.GetBatchWithError(ctx, target...)
	if err != nil {
		panic(err)
	}
}

// GetBatchWithError will try to get the requested dependencies from the DependencyContext. If it
// fails to do so it will return an error. This can still panic due to static issues such as
// if the target is not a pointer to something to be filled.
func (d *DependencyContext) GetBatchWithError(ctx context.Context, targets ...any) error {
	for _, target := range targets {
		err := d.FillDependency(ctx, target)
		if err != nil {
			return err
		}
	}
	return nil
}

// FillDependency fills in the value of the target, or returns an error if it cannot.
func (d *DependencyContext) FillDependency(ctx context.Context, target any) error {
	s, t, err := d.findApplicableSlot(target)
	if err != nil {
		pdc := d.parentDependencyContext()
		if pdc != nil {
			err = pdc.GetBatchWithError(ctx, target)
			if err == nil {
				// Hoist the parent dependency to this level to save time on future calls.
				// At this point the target is a pointer to a pointer to the value, so we
				// have to unwrap one level of indirection.
				d.slots.Store(t, &slot{
					value:     reflect.ValueOf(target).Elem().Interface(),
					generator: nil,
					slotType:  t,
					status:    StatusFromParent,
				})
			}
		}
		return err
	}

	err = d.getValue(ctx, s, t, target)
	if err != nil {
		return err
	}
	return nil
}

// hasApplicableDependency returns if this, or a parent dependency context, as a slot that
// can fulfil that dependency.
func (d *DependencyContext) hasApplicableDependency(target any) bool {
	s, _, _ := d.findApplicableSlot(target)
	if s != nil {
		return true
	}
	pdc := d.parentDependencyContext()
	if pdc != nil {
		return pdc.hasApplicableDependency(target)
	}
	return false
}

// findApplicableSlot looks for an appropriate slot that can fulfil the requested target. If
// the slot is directly found by the request type, simply return it. Otherwise, look for another
// slot that can be assigned to the target and return that if fount. Returns nil if
// nothing is suitable.
func (d *DependencyContext) findApplicableSlot(target any) (*slot, reflect.Type, error) {
	pt := reflect.TypeOf(target)
	if pt.Kind() != reflect.Pointer {
		panic(fmt.Sprintf("target must be a pointer type: %v", pt))
	}
	requestedType := pt.Elem()

	if s, ok := d.slots.Load(requestedType); ok {
		return s.(*slot), requestedType, nil
	}

	var slotTarget reflect.Type
	var s *slot
	found := false
	d.slots.Range(func(slotTargetA, sa any) bool {
		slotTarget = slotTargetA.(reflect.Type)
		s = sa.(*slot)
		if requestedType.Kind() == reflect.Interface && slotTarget.AssignableTo(requestedType) {
			// Create a new reference to this slot showing that this slot is assignable
			// to the target. Essentially this caches the slow lookup of the `AssignableTo`
			// check we just did. This is safe because the slot is still the same slot with
			// the same mutex and all.
			d.slots.Store(requestedType, s)
			found = true
			return false
		}
		return true
	})
	if found {
		return s, requestedType, nil
	}

	return nil, requestedType, &DependencyError{
		Message:        "slot not found for requested type",
		ReferencedType: requestedType,
		Status:         d.Status(),
	}
}

// getValue fills in the target value from this slot. If the value is already there through
// either a direct dependency or if it was previously generated then simply return then. Otherwise,
// either use the generator to make a value or delegate to an upstream DependencyContext.
// The precondition for the function is that the slot's type matches the target such that the
// slot can be assigned to target.
func (d *DependencyContext) getValue(ctx context.Context, activeSlot *slot, targetType reflect.Type, target any) error {
	targetVal := reflect.ValueOf(target)

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

	var timingCtx *timing.Context
	if EnableTiming >= TimingGenerators {
		var complete timing.Complete
		name := fmt.Sprintf("CtxGen(%v)", targetType)
		timingCtx, complete = timing.Start(ctx, name)
		timingCtx.AddDetails("generator", formatGeneratorDebug(activeSlot.generator))
		defer complete()
		ctx = timingCtx
	}
	// Before locking this slot, ensure that we're not in a cyclic dependency. If we are,
	// return an error. Otherwise, the lock call would deadlock.
	cycleCtx, unlocker, err := d.enterSlotProcessing(ctx, activeSlot)
	if unlocker != nil {
		defer unlocker()
	}
	if err != nil {
		return err
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
		if timingCtx != nil {
			timingCtx.AddDetails("wait", "parallel")
		}
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
