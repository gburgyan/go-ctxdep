package ctxdep

import (
	"context"
	"sync"
	"testing"
	"time"
)

type TestCloser struct {
	closed bool
	mu     sync.Mutex
}

func (tc *TestCloser) Close() error {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.closed = true
	return nil
}

func (tc *TestCloser) IsClosed() bool {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	return tc.closed
}

func TestAutoCloseOnContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	closer := &TestCloser{}
	ctx = NewDependencyContext(ctx, WithCleanup(), closer)

	// Verify it's not closed yet
	if closer.IsClosed() {
		t.Error("closer should not be closed yet")
	}

	// Cancel the context
	cancel()

	// Give cleanup goroutine time to run
	time.Sleep(100 * time.Millisecond)

	// Verify it's closed
	if !closer.IsClosed() {
		t.Error("closer should be closed after context cancellation")
	}
}

func TestNoCleanupWithoutOption(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	closer := &TestCloser{}
	// Note: NOT using WithCleanup()
	ctx = NewDependencyContext(ctx, closer)

	// Cancel the context
	cancel()

	// Give time for cleanup (which shouldn't happen)
	time.Sleep(100 * time.Millisecond)

	// Verify it's NOT closed
	if closer.IsClosed() {
		t.Error("closer should not be closed without WithCleanup option")
	}
}

func TestCustomCleanupFunction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	type CustomResource struct {
		cleaned bool
		mu      sync.Mutex
	}

	resource := &CustomResource{}

	cleanupCalled := false
	cleanup := func(r *CustomResource) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.cleaned = true
		cleanupCalled = true
	}

	ctx = NewDependencyContext(ctx,
		WithCleanupFunc(cleanup),
		resource,
	)

	// Cancel the context
	cancel()

	// Give cleanup goroutine time to run
	time.Sleep(100 * time.Millisecond)

	// Verify cleanup was called
	if !cleanupCalled {
		t.Error("cleanup function should have been called")
	}

	resource.mu.Lock()
	cleaned := resource.cleaned
	resource.mu.Unlock()

	if !cleaned {
		t.Error("resource should be marked as cleaned")
	}
}

func TestCleanupOnlyOnce(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	cleanupCount := 0
	var mu sync.Mutex

	type CountedResource struct{}

	cleanup := func(r *CountedResource) {
		mu.Lock()
		defer mu.Unlock()
		cleanupCount++
	}

	resource := &CountedResource{}
	ctx = NewDependencyContext(ctx,
		WithCleanupFunc(cleanup),
		resource,
	)

	// Cancel multiple times
	cancel()
	cancel()

	// Give cleanup goroutine time to run
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	count := cleanupCount
	mu.Unlock()

	if count != 1 {
		t.Errorf("cleanup should be called exactly once, but was called %d times", count)
	}
}

func TestMultipleResourcesCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	type Closer1 struct {
		TestCloser
	}
	type Closer2 struct {
		TestCloser
	}

	closer1 := &Closer1{}
	closer2 := &Closer2{}

	type OtherResource struct {
		cleaned bool
		mu      sync.Mutex
	}

	other := &OtherResource{}

	cleanupOther := func(r *OtherResource) {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.cleaned = true
	}

	ctx = NewDependencyContext(ctx,
		WithCleanup(), // Enable cleanup for io.Closer types
		WithCleanupFunc(cleanupOther),
		closer1,
		closer2,
		other,
	)

	// Cancel the context
	cancel()

	// Give cleanup goroutine time to run
	time.Sleep(100 * time.Millisecond)

	// Verify all resources were cleaned up
	if !closer1.IsClosed() {
		t.Error("closer1 should be closed")
	}
	if !closer2.IsClosed() {
		t.Error("closer2 should be closed")
	}

	other.mu.Lock()
	cleaned := other.cleaned
	other.mu.Unlock()

	if !cleaned {
		t.Error("other resource should be cleaned")
	}
}

func TestGeneratedDependencyCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	generatedCloser := &TestCloser{}

	generator := func() *TestCloser {
		return generatedCloser
	}

	ctx = NewDependencyContext(ctx, WithCleanup(), generator)

	// Trigger generation
	_ = Get[*TestCloser](ctx)

	// Cancel the context
	cancel()

	// Give cleanup goroutine time to run
	time.Sleep(100 * time.Millisecond)

	// Verify generated dependency was cleaned up
	if !generatedCloser.IsClosed() {
		t.Error("generated closer should be closed")
	}
}

func TestNestedContextCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	parentCloser := &TestCloser{}
	childCloser := &TestCloser{}

	// Create parent context
	ctx = NewDependencyContext(ctx, WithCleanup(), parentCloser)

	// Create child context
	childCtx := NewDependencyContext(ctx, WithCleanup(), childCloser)

	// Cancel the parent context
	cancel()

	// Give cleanup goroutines time to run
	time.Sleep(100 * time.Millisecond)

	// Both should be cleaned up
	if !parentCloser.IsClosed() {
		t.Error("parent closer should be closed")
	}
	if !childCloser.IsClosed() {
		t.Error("child closer should be closed")
	}

	// Ensure we can still use childCtx for retrieval even after cleanup
	retrieved := Get[*TestCloser](childCtx)
	if retrieved != childCloser {
		t.Error("should still be able to retrieve from child context")
	}
}
