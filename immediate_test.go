package ctxdep

import (
	"context"
	"fmt"
	"github.com/gburgyan/go-timing"
	"github.com/stretchr/testify/assert"
	"sync/atomic"
	"testing"
	"time"
)

func Test_ImmediateDependency(t *testing.T) {
	var callCount int64
	f := func() *testWidget {
		atomic.AddInt64(&callCount, 1)
		return &testWidget{Val: 42}
	}

	assert.Equal(t, int64(0), atomic.LoadInt64(&callCount))

	ctx := NewDependencyContext(context.Background(), Immediate(f))

	// Wait a bit to ensure the goroutine completes.
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, int64(1), atomic.LoadInt64(&callCount))

	var widget *testWidget
	GetBatch(ctx, &widget)

	assert.Equal(t, 42, widget.Val)
	assert.Equal(t, int64(1), atomic.LoadInt64(&callCount))
}

func Test_ImmediateDependency_LongCall(t *testing.T) {
	var callCount int64
	f := func() *testWidget {
		atomic.AddInt64(&callCount, 1)
		time.Sleep(100 * time.Millisecond)
		return &testWidget{Val: 42}
	}

	assert.Equal(t, int64(0), atomic.LoadInt64(&callCount))

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
	assert.Equal(t, int64(1), atomic.LoadInt64(&callCount))
	assert.InEpsilon(t, 50*time.Millisecond, d, .1)
}

func Test_ImmediateDependency_Error(t *testing.T) {
	var callCount int64
	f := func() (*testWidget, error) {
		atomic.AddInt64(&callCount, 1)
		return nil, fmt.Errorf("expected error")
	}

	assert.Equal(t, int64(0), atomic.LoadInt64(&callCount))

	ctx := NewDependencyContext(context.Background(), Immediate(f))

	// Wait a bit to ensure the goroutine completes.
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, int64(1), atomic.LoadInt64(&callCount))

	var widget *testWidget
	assert.Panics(t, func() {
		GetBatch(ctx, &widget)
	})

	assert.Nil(t, widget)
	assert.Equal(t, int64(2), atomic.LoadInt64(&callCount))
}

func Test_ImmediateDependency_Panic(t *testing.T) {
	var callCount int64
	f := func() (*testWidget, error) {
		atomic.AddInt64(&callCount, 1)
		panic("expected panic")
	}

	assert.Equal(t, int64(0), atomic.LoadInt64(&callCount))

	ctx := NewDependencyContext(context.Background(), Immediate(f))

	// Wait a bit to ensure the goroutine completes.
	time.Sleep(100 * time.Millisecond)

	assert.Equal(t, int64(1), atomic.LoadInt64(&callCount))

	var widget *testWidget
	assert.Panics(t, func() {
		GetBatch(ctx, &widget)
	})

	assert.Nil(t, widget)
	assert.Equal(t, int64(2), atomic.LoadInt64(&callCount))
}
