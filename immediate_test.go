package ctxdep

import (
	"context"
	"fmt"
	"github.com/gburgyan/go-timing"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func Test_ImmediateDependency(t *testing.T) {
	callCount := 0
	f := func() *testWidget {
		callCount++
		return &testWidget{Val: 42}
	}

	assert.Equal(t, 0, callCount)

	ctx := NewDependencyContext(context.Background(), Immediate(f))

	// Wait a bit to ensure the goroutine completes.
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, 1, callCount)

	var widget *testWidget
	GetBatch(ctx, &widget)

	assert.Equal(t, 42, widget.Val)
	assert.Equal(t, 1, callCount)
}

func Test_ImmediateDependency_LongCall(t *testing.T) {
	callCount := 0
	f := func() *testWidget {
		callCount++
		time.Sleep(100 * time.Millisecond)
		return &testWidget{Val: 42}
	}

	assert.Equal(t, 0, callCount)

	EnableTiming = TimingGenerators
	timingCtx := timing.Root(context.Background())

	ctx := NewDependencyContext(timingCtx, Immediate(f))

	// Delay the start of the real request by 50ms. The immediate call should have locked
	// the slot while it's computing the value (100ms) so _this_ call should take another
	// 250ms.
	time.Sleep(50 * time.Millisecond)
	start := time.Now()

	var widget *testWidget
	GetBatch(ctx, &widget)

	d := time.Since(start)

	assert.Equal(t, 42, widget.Val)
	assert.Equal(t, 1, callCount)
	assert.InEpsilon(t, 50*time.Millisecond, d, .1)

	fmt.Println(timingCtx.String())
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

func Test_ImmediateDependency_Panic(t *testing.T) {
	callCount := 0
	f := func() (*testWidget, error) {
		callCount++
		panic("expected panic")
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
