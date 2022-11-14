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

type tempInterface interface {
	getVal() int
}

type tempImpl struct {
}

func (t tempImpl) getVal() int {
	return 105
}

func TestDependencyContext_GeneratorAndObject(t *testing.T) {
	ctx := NewDependencyContext(context.Background(), func() *testWidget {
		return &testWidget{val: 42}
	}, &tempImpl{})

	var widget *testWidget
	Get(ctx, &widget)

	assert.Equal(t, 42, widget.val)

	var iface tempInterface
	Get(ctx, &iface)
	assert.Equal(t, 105, iface.getVal())
}

func TestDependencyContext_AddGenerator_MultiOutput(t *testing.T) {
	calls := 0
	creator := func(ctx context.Context) (*testWidget, *testDoodad) {
		calls++
		return &testWidget{val: 42}, &testDoodad{val: "new doodad"}
	}

	ctx := NewDependencyContext(context.Background(), creator)

	var doodad *testDoodad
	Get(ctx, &doodad)
	assert.Equal(t, "new doodad", doodad.val)

	var widget *testWidget
	Get(ctx, &widget)
	assert.Equal(t, 42, widget.val)

	assert.Equal(t, 1, calls)
}

func TestDependencyContext_AddGenerator_Error(t *testing.T) {
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

func TestDependencyContext_AddGenerator_MultiRequest(t *testing.T) {
	calls := 0
	creator := func(ctx context.Context) (*testWidget, *testDoodad) {
		calls++
		return &testWidget{val: 42}, &testDoodad{val: "new doodad"}
	}

	ctx := NewDependencyContext(context.Background(), creator)

	var doodad *testDoodad
	var widget *testWidget
	Get(ctx, &doodad, &widget)
	assert.Equal(t, "new doodad", doodad.val)
	assert.Equal(t, 42, widget.val)

	assert.Equal(t, 1, calls)
}

func TestDependencyContext_ErrorFromGenerator(t *testing.T) {
	calls := 0
	creator := func(ctx context.Context) (*testWidget, *testDoodad, error) {
		calls++
		return nil, nil, fmt.Errorf("expected error")
	}

	ctx := NewDependencyContext(context.Background(), creator)

	var doodad *testDoodad
	var widget *testWidget
	assert.Panics(t, func() {
		Get(ctx, &doodad, &widget)
	})
}

func TestDependencyContext_RelatedDependencyGenerator(t *testing.T) {
	ctx := NewDependencyContext(context.Background(), func(ctx context.Context) *tempImpl {
		return &tempImpl{}
	})

	var iface tempInterface
	Get(ctx, &iface)

	assert.Equal(t, 105, iface.getVal())
}

func TestMultiLevelDeps(t *testing.T) {
	f1 := func(ctx context.Context) *testWidget {
		return &testWidget{
			val: 42,
		}
	}

	f2 := func(ctx context.Context, widget *testWidget) *testDoodad {
		return &testDoodad{
			val: fmt.Sprintf("%d", widget.val),
		}
	}

	ctx := NewDependencyContext(context.Background(), f1, f2)

	var doodad *testDoodad
	Get(ctx, &doodad)

	assert.Equal(t, "42", doodad.val)
}

func TestCyclicDependencies_params(t *testing.T) {
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

	var widget *testWidget
	err := GetWithError(ctx, &widget)

	assert.Error(t, err)
	assert.Equal(t, "cyclic dependency error getting slot: *ctxdep.testWidget", err.Error())
}

func TestCyclicDependencies_implicit(t *testing.T) {
	f1 := func(ctx context.Context) (*testWidget, error) {
		var doodad *testDoodad
		err := GetWithError(ctx, &doodad)
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
		err := GetWithError(ctx, &widget)
		if err != nil {
			return nil, err
		}
		return &testDoodad{
			val: fmt.Sprintf("%d", widget.val),
		}, nil
	}

	ctx := NewDependencyContext(context.Background(), f1, f2)

	var widget *testWidget
	err := GetWithError(ctx, &widget)

	assert.Error(t, err)
	// Note how this is different from the previous error. The control has passed from the
	// dependency context to the calling function because the dependency engine cannot know
	// what the function does. The errors are returned from the generator function and wrapped
	// by each error.
	assert.Equal(t, "error running generator: *ctxdep.testWidget (error running generator: *ctxdep.testDoodad (cyclic dependency error getting slot: *ctxdep.testWidget))", err.Error())
}

func TestMultiLevelDependencies(t *testing.T) {
	c1 := NewDependencyContext(context.Background(), func() *testWidget { return &testWidget{val: 42} })

	c2 := NewDependencyContext(c1, func(w *testWidget) *testDoodad { return &testDoodad{val: fmt.Sprintf("Doodad: %d", w.val)} })

	var doodad *testDoodad
	Get(c2, &doodad)

	assert.Equal(t, "Doodad: 42", doodad.val)
	assert.Equal(t, "*ctxdep.testDoodad - value: true - generator: (*ctxdep.testWidget) *ctxdep.testDoodad\n----\nparent dependency context:\n*ctxdep.testWidget - value: true - generator: () *ctxdep.testWidget", Status(c2))
}

// While the intent is to store pointers to objects and not objects themselves to prevent copying,
// verify that objects work as well.
func TestNonPointerDependencies(t *testing.T) {
	ctx := NewDependencyContext(context.Background(), func() testWidget { return testWidget{val: 42} })

	var widget testWidget
	Get(ctx, &widget)

	assert.Equal(t, 42, widget.val)
}

func TestInvalidMultipleGenerators(t *testing.T) {
	f1 := func() (*testWidget, *testDoodad) { return nil, nil }
	f2 := func() *testDoodad { return nil }

	// Two functions cannot return the same type, in this case the *testDoodad
	assert.Panics(t, func() {
		_ = NewDependencyContext(context.Background(), f1, f2)
	})
}

func TestGeneratorReturnNilError(t *testing.T) {
	f := func() *testDoodad { return nil }
	ctx := NewDependencyContext(context.Background(), f)

	var doodad *testDoodad
	err := GetWithError(ctx, &doodad)
	assert.Error(t, err)
	assert.Equal(t, "error mapping generator results to context: *ctxdep.testDoodad (generator returned nil result: *ctxdep.testDoodad)", err.Error())
}
