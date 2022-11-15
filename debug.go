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

	for _, s := range d.slots {
		keyString := fmt.Sprintf("%v", s.slotType)
		slotVals[keyString] = fmt.Sprintf("%v - value: %t - generator: %s", s.slotType, s.value != nil, formatGeneratorDebug(s.generator))
		slotKeys = append(slotKeys, keyString)
	}

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
