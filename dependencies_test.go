package ctxdep

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"strconv"
	"testing"
)

type testWidget struct {
	val int
}

type testDoodad struct {
	val string
}

type testInterface interface {
	getVal() int
}

type testImpl struct {
	val int
}

func (t *testImpl) getVal() int {
	return t.val
}

func Test_SimpleObject(t *testing.T) {
	ctx := NewDependencyContext(context.Background(), &testWidget{val: 42})

	var widget *testWidget
	GetBatch(ctx, &widget)
	assert.Equal(t, 42, widget.val)

	dc := GetDependencyContext(ctx)
	widget = nil
	dc.GetBatch(ctx, &widget)
	assert.Equal(t, 42, widget.val)

	assert.Equal(t, "*ctxdep.testWidget - direct value set", Status(ctx))
}

func Test_SimpleObjectGeneric(t *testing.T) {
	ctx := NewDependencyContext(context.Background(), &testWidget{val: 42})

	widget := Get[*testWidget](ctx)
	assert.Equal(t, 42, widget.val)

	assert.Equal(t, "*ctxdep.testWidget - direct value set", Status(ctx))
}

func Test_GeneratorAndObject(t *testing.T) {
	ctx := NewDependencyContext(context.Background(), func() *testWidget {
		return &testWidget{val: 42}
	}, &testImpl{val: 105})

	assert.Equal(t, "*ctxdep.testImpl - direct value set\n*ctxdep.testWidget - uninitialized - generator: () *ctxdep.testWidget", Status(ctx))

	widget := Get[*testWidget](ctx)
	assert.Equal(t, 42, widget.val)

	var iface testInterface
	GetBatch(ctx, &iface)
	assert.Equal(t, 105, iface.getVal())

	assert.Equal(t, "*ctxdep.testImpl - direct value set\n*ctxdep.testWidget - created from generator: () *ctxdep.testWidget\nctxdep.testInterface - assigned from *ctxdep.testImpl", Status(ctx))
}

func Test_AddGenerator_MultiOutput(t *testing.T) {
	calls := 0
	creator := func(ctx context.Context) (*testWidget, *testDoodad) {
		calls++
		return &testWidget{val: 42}, &testDoodad{val: "new doodad"}
	}

	ctx := NewDependencyContext(context.Background(), creator)

	doodad := Get[*testDoodad](ctx)
	assert.Equal(t, "new doodad", doodad.val)

	widget := Get[*testWidget](ctx)
	assert.Equal(t, 42, widget.val)

	assert.Equal(t, 1, calls)
}

func Test_AddGenerator_OnlyError(t *testing.T) {
	// No valid return types
	assert.Panics(t, func() {
		NewDependencyContext(context.Background(), func(ctx context.Context) error {
			return fmt.Errorf("expected error")
		})
	})

	// Multiple errors
	assert.Panics(t, func() {
		NewDependencyContext(context.Background(), func() (*testWidget, error, error) {
			return nil, nil, nil
		})
	})
}

func Test_Generator_MultipleRequests(t *testing.T) {
	calls := 0
	creator := func(ctx context.Context) (*testWidget, *testDoodad) {
		calls++
		return &testWidget{val: 42}, &testDoodad{val: "new doodad"}
	}

	ctx := NewDependencyContext(context.Background(), creator)

	var doodad *testDoodad
	var widget *testWidget
	GetBatch(ctx, &doodad, &widget)
	assert.Equal(t, "new doodad", doodad.val)
	assert.Equal(t, 42, widget.val)

	assert.Equal(t, 1, calls)
}

func Test_GeneratorWithError_Error(t *testing.T) {
	calls := 0
	creator := func(ctx context.Context) (*testWidget, *testDoodad, error) {
		calls++
		return nil, nil, fmt.Errorf("expected error")
	}

	ctx := NewDependencyContext(context.Background(), creator)

	var doodad *testDoodad
	var widget *testWidget
	assert.Panics(t, func() {
		GetBatch(ctx, &doodad, &widget)
	})

	dc := GetDependencyContext(ctx)
	assert.Panics(t, func() {
		dc.GetBatch(ctx, &doodad, &widget)
	})
}

func Test_GeneratorWithError_NoError(t *testing.T) {
	calls := 0
	creator := func(ctx context.Context) (*testWidget, *testDoodad, error) {
		calls++
		return &testWidget{val: 42}, &testDoodad{val: "myval"}, nil
	}

	ctx := NewDependencyContext(context.Background(), creator)

	var doodad *testDoodad
	var widget *testWidget
	GetBatch(ctx, &doodad, &widget)
	assert.Equal(t, 42, widget.val)
	assert.Equal(t, "myval", doodad.val)

	assert.Equal(t, "*ctxdep.testDoodad - created from generator: (context.Context) *ctxdep.testWidget, *ctxdep.testDoodad, error\n*ctxdep.testWidget - created from generator: (context.Context) *ctxdep.testWidget, *ctxdep.testDoodad, error", Status(ctx))
}

func Test_RelatedInterfaceGenerator(t *testing.T) {
	ctx := NewDependencyContext(context.Background(), func(ctx context.Context) *testImpl {
		return &testImpl{val: 105}
	})

	iface := Get[testInterface](ctx)

	assert.Equal(t, 105, iface.getVal())
}

func Test_MultiLevelDependencies(t *testing.T) {
	f1 := func(ctx context.Context) *testWidget {
		return &testWidget{
			val: 42,
		}
	}

	// Create another dependency context that is also on the context stack.
	f2 := func(ctx context.Context, widget *testWidget) *testDoodad {
		return &testDoodad{
			val: fmt.Sprintf("%d", widget.val),
		}
	}

	ctx := NewDependencyContext(context.Background(), f1, f2)

	doodad := Get[*testDoodad](ctx)

	assert.Equal(t, "42", doodad.val)
	assert.Equal(t, "*ctxdep.testDoodad - created from generator: (context.Context, *ctxdep.testWidget) *ctxdep.testDoodad\n*ctxdep.testWidget - created from generator: (context.Context) *ctxdep.testWidget", Status(ctx))
}

func Test_CyclicDependencies_FromParams(t *testing.T) {
	f1 := func(ctx context.Context, doodad *testDoodad) *testWidget {
		val, _ := strconv.Atoi(doodad.val)
		return &testWidget{
			val: val,
		}
	}

	f2 := func(ctx context.Context, widget *testWidget) *testDoodad {
		return &testDoodad{
			val: fmt.Sprintf("%d", widget.val),
		}
	}

	ctx := NewDependencyContext(context.Background(), f1, f2)

	_, err := GetWithError[*testWidget](ctx)

	assert.Error(t, err)
	assert.Equal(t, "cyclic dependency error getting slot: *ctxdep.testWidget", err.Error())
}

func Test_CyclicDependencies_Implicit(t *testing.T) {
	f1 := func(ctx context.Context) (*testWidget, error) {
		var doodad *testDoodad
		err := GetBatchWithError(ctx, &doodad)
		if err != nil {
			return nil, err
		}
		val, _ := strconv.Atoi(doodad.val)
		return &testWidget{
			val: val,
		}, nil
	}

	f2 := func(ctx context.Context) (*testDoodad, error) {
		var widget *testWidget
		err := GetBatchWithError(ctx, &widget)
		if err != nil {
			return nil, err
		}
		return &testDoodad{
			val: fmt.Sprintf("%d", widget.val),
		}, nil
	}

	ctx := NewDependencyContext(context.Background(), f1, f2)

	_, err := GetWithError[*testWidget](ctx)

	assert.Error(t, err)
	// Note how this is different from the previous error. The control has passed from the
	// dependency context to the calling function because the dependency engine cannot know
	// what the function does. The errors are returned from the generator function and wrapped
	// by each error.
	assert.Equal(t, "error running generator: *ctxdep.testWidget (error running generator: *ctxdep.testDoodad (cyclic dependency error getting slot: *ctxdep.testWidget))", err.Error())
}

func Test_MultiLevelDependencies_Param(t *testing.T) {
	c1 := NewDependencyContext(context.Background(), func() *testWidget { return &testWidget{val: 42} })

	c2 := NewDependencyContext(c1, func(w *testWidget) *testDoodad { return &testDoodad{val: fmt.Sprintf("Doodad: %d", w.val)} })

	doodad := Get[*testDoodad](c2)

	assert.Equal(t, "Doodad: 42", doodad.val)
	assert.Equal(t, "*ctxdep.testDoodad - created from generator: (*ctxdep.testWidget) *ctxdep.testDoodad\n*ctxdep.testWidget - imported from parent context\n----\nparent dependency context:\n*ctxdep.testWidget - created from generator: () *ctxdep.testWidget", Status(c2))
}

// While the intent is to store pointers to objects and not objects themselves to prevent copying,
// verify that objects work as well.
func Test_NonPointerDependencies(t *testing.T) {
	ctx := NewDependencyContext(context.Background(), func() testWidget { return testWidget{val: 42} })

	widget := Get[testWidget](ctx)

	assert.Equal(t, 42, widget.val)
	assert.Equal(t, "ctxdep.testWidget - created from generator: () ctxdep.testWidget", Status(ctx))
}

func Test_MultipleGenerators_Invalid(t *testing.T) {
	f1 := func() (*testWidget, *testDoodad) { return nil, nil }
	f2 := func() *testDoodad { return nil }

	// Two functions cannot return the same type, in this case the *testDoodad
	assert.Panics(t, func() {
		_ = NewDependencyContext(context.Background(), f1, f2)
	})
}

func Test_MultipleGenerators_UnresolvedDependency(t *testing.T) {
	f := func(_ *testDoodad) *testWidget { return nil }

	// The function needs a testDoodad, but there is no such thing in the context.
	assert.Panics(t, func() {
		_ = NewDependencyContext(context.Background(), f)
	})
}

func Test_GeneratorReturnNil(t *testing.T) {
	f := func() *testDoodad { return nil }
	ctx := NewDependencyContext(context.Background(), f)

	_, err := GetWithError[*testDoodad](ctx)
	assert.Error(t, err)
	assert.Equal(t, "error mapping generator results to context: *ctxdep.testDoodad (generator returned nil result: *ctxdep.testDoodad)", err.Error())
}

func Test_UnknownDependencies(t *testing.T) {
	ctx := NewDependencyContext(context.Background(), func() testWidget { return testWidget{val: 42} })

	_, err := GetWithError[*testDoodad](ctx)
	assert.Error(t, err)
	assert.Equal(t, "slot not found for requested type: *ctxdep.testDoodad", err.Error())
}

func Test_NoDependencyContext(t *testing.T) {
	ctx := context.Background()
	var widget *testWidget
	assert.Panics(t, func() {
		GetBatch(ctx, &widget)
	})
}

func Test_AddNilDependency(t *testing.T) {
	var nilWidget *testWidget
	assert.Panics(t, func() {
		NewDependencyContext(context.Background(), nilWidget)
	})
}

func Test_NonPointerGet(t *testing.T) {
	ctx := NewDependencyContext(context.Background(), func() testWidget { return testWidget{val: 42} })

	var doodad testDoodad
	assert.Panics(t, func() {
		GetBatch(ctx, doodad)
	})
}

// Verify that we can insert nominally the same type of object, int in this case, that have
// been given unique types themselves.
func Test_TypedObjects(t *testing.T) {
	type intA int
	type intB int

	a := intA(42)
	b := intB(105)
	ctx := NewDependencyContext(context.Background(), &a, &b)

	resultA := Get[*intA](ctx)
	resultB := Get[*intB](ctx)

	assert.Equal(t, intA(42), *resultA)
	assert.Equal(t, intB(105), *resultB)

	assert.Equal(t, "*ctxdep.intA - direct value set\n*ctxdep.intB - direct value set", Status(ctx))
}

func Test_ComplicatedStatus(t *testing.T) {
	// Set up a parent context that returns a concrete implementation of an interface
	c1 := NewDependencyContext(context.Background(), func() *testImpl {
		return &testImpl{val: 42}
	}, func() *testDoodad {
		return &testDoodad{val: "wo0t"}
	})

	// Make another status from that one
	c2 := NewDependencyContext(c1, func(in testInterface) *testWidget {
		return &testWidget{val: in.getVal()}
	}, &testDoodad{val: "something cool"})

	widget := Get[*testWidget](c2)

	assert.Equal(t, 42, widget.val)
	expected := `*ctxdep.testDoodad - direct value set
*ctxdep.testWidget - created from generator: (ctxdep.testInterface) *ctxdep.testWidget
ctxdep.testInterface - imported from parent context
----
parent dependency context:
*ctxdep.testDoodad - uninitialized - generator: () *ctxdep.testDoodad
*ctxdep.testImpl - created from generator: () *ctxdep.testImpl
ctxdep.testInterface - assigned from *ctxdep.testImpl`
	assert.Equal(t, expected, Status(c2))
}
