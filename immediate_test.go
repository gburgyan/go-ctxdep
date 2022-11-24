package ctxdep

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func Test_ImmediateDependency(t *testing.T) {
	callCount := 0
	f := func() *testWidget {
		callCount++
		return &testWidget{val: 42}
	}

	assert.Equal(t, 0, callCount)

	ctx := NewDependencyContext(context.Background(), Immediate(f))

	// Wait a bit to ensure the goroutine completes.
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, 1, callCount)

	var widget *testWidget
	GetBatch(ctx, &widget)

	assert.Equal(t, 42, widget.val)
	assert.Equal(t, 1, callCount)
}

func Test_ImmediateDependency_LongCall(t *testing.T) {
	callCount := 0
	f := func() *testWidget {
		callCount++
		time.Sleep(100 * time.Millisecond)
		return &testWidget{val: 42}
	}

	assert.Equal(t, 0, callCount)

	ctx := NewDependencyContext(context.Background(), Immediate(f))

	// Delay the start of the real request by 50ms. The immediate call should have locked
	// the slot while it's computing the value (100ms) so _this_ call should take another
	// 250ms.
	time.Sleep(50 * time.Millisecond)
	start := time.Now()

	var widget *testWidget
	GetBatch(ctx, &widget)

	d := time.Since(start)

	assert.Equal(t, 42, widget.val)
	assert.Equal(t, 1, callCount)
	assert.InEpsilon(t, 50*time.Millisecond, d, .1)
}

func Test_ImmediateDependency_Error(t *testing.T) {
	callCount := 0
	f := func() (*testWidget, error) {
		callCount++
		return nil, fmt.Errorf("expected error")
	}

	assert.Equal(t, 0, callCount)

	ctx := NewDependencyContext(context.Background(), Immediate(f))

	// Wait a bit to ensure the goroutine completes.
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, 1, callCount)

	var widget *testWidget
	assert.Panics(t, func() {
		GetBatch(ctx, &widget)
	})

	assert.Nil(t, widget)
	assert.Equal(t, 2, callCount)
}

func Test_ImmediateDependencyMutator(t *testing.T) {
	callCount := 0
	f := func() *testWidget {
		callCount++
		return &testWidget{val: 42}
	}
	var contextType string
	m := func(ctx context.Context, ct string) context.Context {
		contextType = ct
		return ctx
	}

	assert.Equal(t, 0, callCount)

	ctx := NewDependencyContext(context.Background(), ImmediateCtxMutator(m, f))

	// Wait a bit to ensure the goroutine completes.
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, 1, callCount)
	assert.Equal(t, "*ctxdep.testWidget", contextType)

	var widget *testWidget
	GetBatch(ctx, &widget)

	assert.Equal(t, 42, widget.val)
	assert.Equal(t, 1, callCount)
}
