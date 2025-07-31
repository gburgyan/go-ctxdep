package ctxdep

import (
	"context"
	"sync"
	"testing"
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

func TestExplicitCleanup(t *testing.T) {
	ctx := context.Background()

	closer := &TestCloser{}
	dc := NewDependencyContext(ctx, WithCleanup(), closer)

	// Verify it's not closed yet
	if closer.IsClosed() {
		t.Error("closer should not be closed yet")
	}

	// Explicitly call cleanup
	dc.Cleanup()

	// Verify it's closed
	if !closer.IsClosed() {
		t.Error("closer should be closed after explicit cleanup")
	}
}

func TestNoCleanupWithoutOption(t *testing.T) {
	ctx := context.Background()

	closer := &TestCloser{}
	// Note: NOT using WithCleanup()
	dc := NewDependencyContext(ctx, closer)

	// Call cleanup
	dc.Cleanup()

	// Verify it's NOT closed
	if closer.IsClosed() {
		t.Error("closer should not be closed without WithCleanup option")
	}
}

func TestCustomCleanupFunction(t *testing.T) {
	ctx := context.Background()

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

	dc := NewDependencyContext(ctx,
		WithCleanupFunc(cleanup),
		resource,
	)

	// Explicitly call cleanup
	dc.Cleanup()

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
	ctx := context.Background()

	cleanupCount := 0
	var mu sync.Mutex

	type CountedResource struct{}

	cleanup := func(r *CountedResource) {
		mu.Lock()
		defer mu.Unlock()
		cleanupCount++
	}

	resource := &CountedResource{}
	dc := NewDependencyContext(ctx,
		WithCleanupFunc(cleanup),
		resource,
	)

	// Call cleanup multiple times
	dc.Cleanup()
	dc.Cleanup()

	mu.Lock()
	count := cleanupCount
	mu.Unlock()

	if count != 1 {
		t.Errorf("cleanup should be called exactly once, but was called %d times", count)
	}
}

func TestMultipleResourcesCleanup(t *testing.T) {
	ctx := context.Background()

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

	dc := NewDependencyContext(ctx,
		WithCleanup(), // Enable cleanup for io.Closer types
		WithCleanupFunc(cleanupOther),
		closer1,
		closer2,
		other,
	)

	// Explicitly call cleanup
	dc.Cleanup()

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
	ctx := context.Background()

	generatedCloser := &TestCloser{}

	generator := func() *TestCloser {
		return generatedCloser
	}

	dc := NewDependencyContext(ctx, WithCleanup(), generator)

	// Trigger generation
	_ = Get[*TestCloser](dc)

	// Explicitly call cleanup
	dc.Cleanup()

	// Verify generated dependency was cleaned up
	if !generatedCloser.IsClosed() {
		t.Error("generated closer should be closed")
	}
}

func TestNestedContextCleanup(t *testing.T) {
	ctx := context.Background()

	parentCloser := &TestCloser{}
	childCloser := &TestCloser{}

	// Create parent context
	parentCtx := NewDependencyContext(ctx, WithCleanup(), parentCloser)

	// Create child context
	childCtx := NewDependencyContext(parentCtx, WithCleanup(), WithOverrides(), childCloser)

	// Cleanup child first
	childCtx.Cleanup()

	// Only child should be cleaned up
	if parentCloser.IsClosed() {
		t.Error("parent closer should not be closed yet")
	}
	if !childCloser.IsClosed() {
		t.Error("child closer should be closed")
	}

	// Cleanup parent
	parentCtx.Cleanup()

	// Now parent should be cleaned up
	if !parentCloser.IsClosed() {
		t.Error("parent closer should be closed")
	}

	// Ensure we can still use childCtx for retrieval even after cleanup
	retrieved := Get[*TestCloser](childCtx)
	if retrieved != childCloser {
		t.Error("should still be able to retrieve from child context")
	}
}

func TestDeferCleanupPattern(t *testing.T) {
	// This test demonstrates the recommended pattern
	testFunc := func() (closed bool) {
		ctx := context.Background()
		closer := &TestCloser{}

		dc := NewDependencyContext(ctx, WithCleanup(), closer)
		defer dc.Cleanup()

		// Use the dependencies
		retrieved := Get[*TestCloser](dc)
		if retrieved != closer {
			t.Error("failed to retrieve dependency")
		}

		// When function returns, cleanup will be called
		return closer.IsClosed()
	}

	// Before function returns, should not be closed
	closed := testFunc()

	// After function returns (and defer runs), should be closed
	if closed {
		t.Error("resource should have been closed by defer")
	}

	// Need to check the closer after the function returns
	// Let's modify this test to be more clear
	closer := &TestCloser{}
	func() {
		ctx := context.Background()
		dc := NewDependencyContext(ctx, WithCleanup(), closer)
		defer dc.Cleanup()

		// Use the dependencies
		_ = Get[*TestCloser](dc)

		// Should not be closed yet
		if closer.IsClosed() {
			t.Error("resource should not be closed before function returns")
		}
	}()

	// After function returns, should be closed
	if !closer.IsClosed() {
		t.Error("resource should be closed after defer cleanup")
	}
}
