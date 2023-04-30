package ctxdep

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Status is a diagnostic tool that returns a string describing the state of the dependency
// context. The result is each dependency type that is known about, and if it has a value
// and if it has a generator that is capable of making that value.
//
// Note that while everything that is returned is true, if a type implements an interface
// or can be cast to another type, and that type hasn't been asked for yet, the other
// type is not yet known.
func (d *DependencyContext) Status() string {
	slotVals := map[string]string{}
	var slotKeys []string

	d.slots.Range(func(key, value any) bool {
		t := key.(reflect.Type)
		s := value.(*slot)
		keyString := fmt.Sprintf("%v", t)
		if t == s.slotType {
			var slotLine string
			switch s.status {
			case StatusDirect:
				slotLine = fmt.Sprintf("%v - direct value set", t)
			case StatusGenerator:
				if s.value == nil {
					slotLine = fmt.Sprintf("%v - uninitialized - generator: %s", t, formatGeneratorDebug(s.generator))
				} else {
					slotLine = fmt.Sprintf("%v - created from generator: %s", t, formatGeneratorDebug(s.generator))
				}
			case StatusFromParent:
				slotLine = fmt.Sprintf("%v - imported from parent context", t)
			}
			// original slots have matching keys and slot types
			slotVals[keyString] = slotLine
		} else {
			// non-matching keys and slot types are created when there is a fuzzier
			// match between the actual slot type and the requested type. These are
			// created lazily in findApplicableSlot.
			slotVals[keyString] = fmt.Sprintf("%v - assigned from %v", t, s.slotType)
		}
		slotKeys = append(slotKeys, keyString)
		return true
	})

	sort.Strings(slotKeys)

	result := strings.Builder{}
	for _, slotKey := range slotKeys {
		if result.Len() > 0 {
			result.WriteString("\n")
		}
		result.WriteString(slotVals[slotKey])
	}

	pdc := d.parentDependencyContext()
	if pdc != nil {
		result.WriteString("\n----\nparent dependency context:\n")
		result.WriteString(pdc.Status())
	}

	return result.String()
}

// formatGeneratorDebug simply returns a string representation of a generator. This is
// used instead of the native `%#v` formatter to not return the raw address of the generator
// as that's not important for this and simplifies testing.
func formatGeneratorDebug(gen any) string {
	if gen == nil {
		return "-"
	}
	genType := reflect.TypeOf(gen)
	if genType.Kind() != reflect.Func {
		// We should never get here
		return "non-function!"
	}
	builder := strings.Builder{}
	builder.WriteString("(")
	for i := 0; i < genType.NumIn(); i++ {
		if i > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(genType.In(i).String())
	}
	builder.WriteString(") ")
	for i := 0; i < genType.NumOut(); i++ {
		if i > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(genType.Out(i).String())
	}
	return builder.String()
}
