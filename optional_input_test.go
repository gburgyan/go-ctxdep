package ctxdep

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test types specific to optional tests
type optTestWidget struct {
	Val int
}

type optTestDoodad struct {
	Val string
}

type optTestInterface interface {
	getValue() int
}

type optTestImpl struct {
	val int
}

func (o *optTestImpl) getValue() int {
	return o.val
}

// TestOptional_NonNilPointer verifies that non-nil pointers are added normally
func TestOptional_NonNilPointer(t *testing.T) {
	widget := &optTestWidget{Val: 42}
	ctx := NewDependencyContext(context.Background(), Optional(widget))

	result := Get[*optTestWidget](ctx)
	assert.NotNil(t, result)
	assert.Equal(t, 42, result.Val)
}

// TestOptional_TypedNilSkipped verifies that typed nil pointers are silently skipped
func TestOptional_TypedNilSkipped(t *testing.T) {
	var nilWidget *optTestWidget

	// This should NOT panic
	ctx := NewDependencyContext(context.Background(), Optional(nilWidget))

	// The dependency should not be found
	_, found := GetOptional[*optTestWidget](ctx)
	assert.False(t, found, "nil optional should not be added to context")
}

// TestOptional_UntypedNilSkipped verifies that untyped nil is silently skipped
func TestOptional_UntypedNilSkipped(t *testing.T) {
	// This should NOT panic
	ctx := NewDependencyContext(context.Background(), Optional(nil))

	// Context should be created successfully
	assert.NotNil(t, ctx)
}

// TestOptional_GeneratorPanics verifies that generators cannot be optional
func TestOptional_GeneratorPanics(t *testing.T) {
	gen := func() *optTestWidget { return &optTestWidget{Val: 42} }

	assert.PanicsWithValue(t,
		"Optional() cannot wrap a generator function",
		func() {
			NewDependencyContext(context.Background(), Optional(gen))
		})
}

// TestOptional_NonPointerPanics verifies that non-pointer types cause panic
func TestOptional_NonPointerPanics(t *testing.T) {
	value := 42

	assert.PanicsWithValue(t,
		"Optional() requires a pointer or interface type, got: int",
		func() {
			NewDependencyContext(context.Background(), Optional(value))
		})
}

// TestOptional_ImmediateInOptionalPanics verifies that Optional(Immediate(...)) panics
func TestOptional_ImmediateInOptionalPanics(t *testing.T) {
	gen := func() *optTestWidget { return &optTestWidget{Val: 42} }

	assert.PanicsWithValue(t,
		"Optional() cannot wrap Immediate()",
		func() {
			NewDependencyContext(context.Background(), Optional(Immediate(gen)))
		})
}

// TestImmediate_OptionalPanics verifies that Immediate(Optional(...)) panics
func TestImmediate_OptionalPanics(t *testing.T) {
	widget := &optTestWidget{Val: 42}

	assert.PanicsWithValue(t,
		"Immediate() cannot wrap Optional()",
		func() {
			NewDependencyContext(context.Background(), Immediate(Optional(widget)))
		})
}

// TestOptional_OverrideablePanics verifies that Optional(Overrideable(...)) panics
func TestOptional_OverrideablePanics(t *testing.T) {
	widget := &optTestWidget{Val: 42}

	assert.PanicsWithValue(t,
		"Optional() cannot wrap Overrideable()",
		func() {
			NewDependencyContext(context.Background(), Optional(Overrideable(widget)))
		})
}

// TestOverrideable_OptionalPanics verifies that Overrideable(Optional(...)) panics
func TestOverrideable_OptionalPanics(t *testing.T) {
	widget := &optTestWidget{Val: 42}

	assert.PanicsWithValue(t,
		"Overrideable() cannot wrap Optional()",
		func() {
			NewDependencyContext(context.Background(), Overrideable(Optional(widget)))
		})
}

// TestOptional_NestedPanics verifies that Optional(Optional(...)) panics
func TestOptional_NestedPanics(t *testing.T) {
	widget := &optTestWidget{Val: 42}

	assert.PanicsWithValue(t,
		"Optional() cannot wrap another Optional()",
		func() {
			NewDependencyContext(context.Background(), Optional(Optional(widget)))
		})
}

// TestOptional_MixedNilAndNonNil verifies multiple optionals with some nil work correctly
func TestOptional_MixedNilAndNonNil(t *testing.T) {
	var nilWidget *optTestWidget
	doodad := &optTestDoodad{Val: "exists"}

	ctx := NewDependencyContext(context.Background(), Optional(nilWidget), Optional(doodad))

	_, foundWidget := GetOptional[*optTestWidget](ctx)
	assert.False(t, foundWidget, "nil optional should not be added")

	resultDoodad, foundDoodad := GetOptional[*optTestDoodad](ctx)
	assert.True(t, foundDoodad, "non-nil optional should be added")
	assert.Equal(t, "exists", resultDoodad.Val)
}

// TestOptional_WithRegularDeps verifies optionals work with regular dependencies
func TestOptional_WithRegularDeps(t *testing.T) {
	regular := &optTestWidget{Val: 42}
	var optional *optTestDoodad

	ctx := NewDependencyContext(context.Background(), regular, Optional(optional))

	w := Get[*optTestWidget](ctx)
	assert.Equal(t, 42, w.Val)

	_, found := GetOptional[*optTestDoodad](ctx)
	assert.False(t, found)
}

// TestOptional_GeneratorCannotDependOnNil verifies that generators fail validation if they depend on skipped optional
func TestOptional_GeneratorCannotDependOnNil(t *testing.T) {
	var nilWidget *optTestWidget
	gen := func(w *optTestWidget) *optTestDoodad {
		return &optTestDoodad{Val: "depends on widget"}
	}

	_, err := NewDependencyContextWithValidation(context.Background(), Optional(nilWidget), gen)
	assert.Error(t, err, "should fail validation when generator depends on nil optional")
	assert.Contains(t, err.Error(), "dependencies that cannot be resolved")
}

// TestOptional_GeneratorCanDependOnNonNil verifies that generators work when optional is non-nil
func TestOptional_GeneratorCanDependOnNonNil(t *testing.T) {
	widget := &optTestWidget{Val: 42}
	gen := func(w *optTestWidget) *optTestDoodad {
		return &optTestDoodad{Val: "widget val: " + string(rune('0'+w.Val%10))}
	}

	ctx := NewDependencyContext(context.Background(), Optional(widget), gen)

	result := Get[*optTestDoodad](ctx)
	assert.NotNil(t, result)
}

// TestOptional_NilWithoutWrapperStillPanics verifies original behavior is preserved
func TestOptional_NilWithoutWrapperStillPanics(t *testing.T) {
	var nilWidget *optTestWidget

	assert.PanicsWithValue(t,
		"invalid nil value dependency for type *ctxdep.optTestWidget",
		func() {
			NewDependencyContext(context.Background(), nilWidget)
		})
}

// TestOptional_NilInterfaceSkipped verifies nil interface is skipped
func TestOptional_NilInterfaceSkipped(t *testing.T) {
	var nilInterface optTestInterface

	ctx := NewDependencyContext(context.Background(), Optional(nilInterface))

	_, found := GetOptional[optTestInterface](ctx)
	assert.False(t, found, "nil interface optional should not be added")
}

// TestOptional_NonNilInterfaceAdded verifies non-nil interface is added
func TestOptional_NonNilInterfaceAdded(t *testing.T) {
	impl := &optTestImpl{val: 42}
	var iface optTestInterface = impl

	ctx := NewDependencyContext(context.Background(), Optional(iface))

	result, found := GetOptional[optTestInterface](ctx)
	assert.True(t, found)
	assert.Equal(t, 42, result.getValue())
}

// TestOptional_InSlice verifies optional works when passed in a slice
func TestOptional_InSlice(t *testing.T) {
	var nilWidget *optTestWidget
	doodad := &optTestDoodad{Val: "exists"}

	deps := []any{
		Optional(nilWidget),
		Optional(doodad),
	}

	ctx := NewDependencyContext(context.Background(), deps)

	_, foundWidget := GetOptional[*optTestWidget](ctx)
	assert.False(t, foundWidget, "nil optional in slice should not be added")

	resultDoodad, foundDoodad := GetOptional[*optTestDoodad](ctx)
	assert.True(t, foundDoodad, "non-nil optional in slice should be added")
	assert.Equal(t, "exists", resultDoodad.Val)
}

// TestOptional_AdaptPanics verifies that Optional(Adapt(...)) panics
func TestOptional_AdaptPanics(t *testing.T) {
	type MyAdapter func(ctx context.Context) *optTestWidget

	adapted := Adapt[MyAdapter](func(ctx context.Context, d *optTestDoodad) *optTestWidget {
		return &optTestWidget{Val: 1}
	})

	assert.PanicsWithValue(t,
		"Optional() cannot wrap Adapt()",
		func() {
			NewDependencyContext(context.Background(),
				&optTestDoodad{Val: "x"},
				Optional(adapted))
		})
}

// TestOptional_ValidatePanics verifies that Optional(Validate(...)) panics
func TestOptional_ValidatePanics(t *testing.T) {
	validator := Validate(func(ctx context.Context) error {
		return nil
	})

	assert.PanicsWithValue(t,
		"Optional() cannot wrap Validate()",
		func() {
			NewDependencyContext(context.Background(), Optional(validator))
		})
}

// TestOptional_StringType verifies that string type (non-pointer) panics
func TestOptional_StringType(t *testing.T) {
	value := "test"

	assert.PanicsWithValue(t,
		"Optional() requires a pointer or interface type, got: string",
		func() {
			NewDependencyContext(context.Background(), Optional(value))
		})
}

// TestOptional_StructType verifies that struct type (non-pointer) panics
func TestOptional_StructType(t *testing.T) {
	value := optTestWidget{Val: 42}

	assert.PanicsWithValue(t,
		"Optional() requires a pointer or interface type, got: ctxdep.optTestWidget",
		func() {
			NewDependencyContext(context.Background(), Optional(value))
		})
}
