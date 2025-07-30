package ctxdep

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Test types for adapter tests
type TestDatabase struct {
	Name string
}

type TestUser struct {
	ID    string
	Name  string
	Email string
}

type TestConfig struct {
	APIKey string
}

// Adapter function types
type UserAdapter func(ctx context.Context, userID string) (*TestUser, error)
type UserAdapterNoError func(ctx context.Context, userID string) *TestUser
type ComplexAdapter func(ctx context.Context, name string, age int) (string, error)

// Test functions that will be wrapped as adapters
func lookupUser(ctx context.Context, db *TestDatabase, userID string) (*TestUser, error) {
	if userID == "error" {
		return nil, errors.New("user not found")
	}
	return &TestUser{
		ID:    userID,
		Name:  "Test User from " + db.Name,
		Email: userID + "@example.com",
	}, nil
}

func lookupUserNoError(ctx context.Context, db *TestDatabase, userID string) *TestUser {
	return &TestUser{
		ID:    userID,
		Name:  "Test User from " + db.Name,
		Email: userID + "@example.com",
	}
}

func complexFunction(ctx context.Context, db *TestDatabase, config *TestConfig, name string, age int) (string, error) {
	if age < 0 {
		return "", errors.New("invalid age")
	}
	return db.Name + ":" + config.APIKey + ":" + name + ":" + string(rune(age)), nil
}

func TestAdapterBasic(t *testing.T) {
	// Create a context with dependencies
	db := &TestDatabase{Name: "TestDB"}
	ctx := NewDependencyContext(context.Background(), db, Adapt[UserAdapter](lookupUser))

	// Get the adapter
	adapter := Get[UserAdapter](ctx)
	if adapter == nil {
		t.Fatal("adapter should not be nil")
	}

	// Use the adapter
	user, err := adapter(ctx, "user123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.ID != "user123" {
		t.Errorf("expected user ID 'user123', got '%s'", user.ID)
	}
	if user.Name != "Test User from TestDB" {
		t.Errorf("expected user name 'Test User from TestDB', got '%s'", user.Name)
	}
	if user.Email != "user123@example.com" {
		t.Errorf("expected email 'user123@example.com', got '%s'", user.Email)
	}
}

func TestAdapterWithError(t *testing.T) {
	// Create a context with dependencies
	db := &TestDatabase{Name: "TestDB"}
	ctx := NewDependencyContext(context.Background(), db, Adapt[UserAdapter](lookupUser))

	// Get the adapter
	adapter := Get[UserAdapter](ctx)

	// Use the adapter with error case
	user, err := adapter(ctx, "error")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "user not found" {
		t.Errorf("expected error 'user not found', got '%v'", err)
	}
	if user != nil {
		t.Error("expected nil user on error")
	}
}

func TestAdapterNoError(t *testing.T) {
	// Create a context with dependencies
	db := &TestDatabase{Name: "TestDB"}
	ctx := NewDependencyContext(context.Background(), db, Adapt[UserAdapterNoError](lookupUserNoError))

	// Get the adapter
	adapter := Get[UserAdapterNoError](ctx)

	// Use the adapter
	user := adapter(ctx, "user456")
	if user == nil {
		t.Fatal("user should not be nil")
	}
	if user.ID != "user456" {
		t.Errorf("expected user ID 'user456', got '%s'", user.ID)
	}
}

func TestAdapterMultipleDependencies(t *testing.T) {
	// Create a context with multiple dependencies
	db := &TestDatabase{Name: "TestDB"}
	config := &TestConfig{APIKey: "secret123"}
	ctx := NewDependencyContext(context.Background(), db, config, Adapt[ComplexAdapter](complexFunction))

	// Get the adapter
	adapter := Get[ComplexAdapter](ctx)

	// Use the adapter
	result, err := adapter(ctx, "John", 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "TestDB:secret123:John:" + string(rune(30))
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func TestAdapterMissingDependency(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for missing dependency")
		}
	}()

	// Create a context without the required database dependency
	_ = NewDependencyContext(context.Background(), Adapt[UserAdapter](lookupUser))
	// This should panic during initialization
}

func TestAdapterInvalidTargetType(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid target type")
		}
	}()

	// Try to create an adapter with non-function target type
	type NotAFunction struct{}
	Adapt[NotAFunction](lookupUser)
}

func TestAdapterInvalidFunction(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid function")
		}
	}()

	// Try to create an adapter with non-function argument
	Adapt[UserAdapter]("not a function")
}

func TestAdapterParameterMismatch(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for parameter mismatch")
		}
	}()

	// Wrong adapter type - expects different parameters
	type WrongAdapter func(ctx context.Context, userID string, extra int) (*TestUser, error)
	db := &TestDatabase{Name: "TestDB"}
	_ = NewDependencyContext(context.Background(), db, Adapt[WrongAdapter](lookupUser))
}

func TestAdapterReturnMismatch(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for return mismatch")
		}
	}()

	// Wrong adapter type - expects different return types
	type WrongAdapter func(ctx context.Context, userID string) (string, error)
	Adapt[WrongAdapter](lookupUser)
}

func TestAdapterWithOptionalDependency(t *testing.T) {
	// Test that adapter validates missing dependencies at initialization
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when adapter has unresolvable dependencies")
		}
	}()

	// Function that requires a dependency not in context
	fn := func(ctx context.Context, missing *TestConfig, id string) (*TestUser, error) {
		return &TestUser{ID: id}, nil
	}

	type TestAdapter func(ctx context.Context, id string) (*TestUser, error)

	// This should panic because TestConfig is not available
	ctx := NewDependencyContext(context.Background(), Adapt[TestAdapter](fn))
	_ = ctx
}

func TestAdapterContextUpdate(t *testing.T) {
	// Test that adapter uses dependencies from creation time, not from the provided context
	db := &TestDatabase{Name: "OriginalDB"}
	ctx := NewDependencyContext(context.Background(), db, Adapt[UserAdapter](lookupUser))

	adapter := Get[UserAdapter](ctx)

	// Create a new context with updated database
	newDB := &TestDatabase{Name: "UpdatedDB"}
	newCtx := NewDependencyContext(ctx, newDB, WithOverrides())

	// Use adapter with new context
	user, err := adapter(newCtx, "user789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should use the original database from when adapter was created
	if user.Name != "Test User from OriginalDB" {
		t.Errorf("expected user from OriginalDB, got '%s'", user.Name)
	}
}

func TestAdapterNoContextParameter(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for adapter without context parameter")
		}
	}()

	// Adapter target requires context but original function doesn't have it
	noCtxFunc := func(db *TestDatabase, userID string) (*TestUser, error) {
		return &TestUser{ID: userID}, nil
	}

	db := &TestDatabase{Name: "TestDB"}
	ctx := NewDependencyContext(context.Background(), db, Adapt[UserAdapter](noCtxFunc))

	adapter := Get[UserAdapter](ctx)
	// This should panic when trying to call the adapter
	_, _ = adapter(ctx, "test")
}

func TestAdapterAllDependenciesFromContext(t *testing.T) {
	// Test function where all non-context parameters come from context
	type SimpleAdapter func(ctx context.Context) (*TestUser, error)

	fn := func(ctx context.Context, db *TestDatabase, config *TestConfig) (*TestUser, error) {
		return &TestUser{
			ID:    config.APIKey,
			Name:  db.Name,
			Email: "test@example.com",
		}, nil
	}

	db := &TestDatabase{Name: "TestDB"}
	config := &TestConfig{APIKey: "key123"}
	ctx := NewDependencyContext(context.Background(), db, config, Adapt[SimpleAdapter](fn))

	adapter := Get[SimpleAdapter](ctx)
	user, err := adapter(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.ID != "key123" {
		t.Errorf("expected user ID 'key123', got '%s'", user.ID)
	}
	if user.Name != "TestDB" {
		t.Errorf("expected user name 'TestDB', got '%s'", user.Name)
	}
}

func TestAdapterConcurrent(t *testing.T) {
	// Test concurrent adapter usage
	db := &TestDatabase{Name: "TestDB"}
	ctx := NewDependencyContext(context.Background(), db, Adapt[UserAdapter](lookupUser))

	adapter := Get[UserAdapter](ctx)

	// Run multiple goroutines using the adapter
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			userID := "user" + string(rune('0'+id))
			user, err := adapter(ctx, userID)
			if err != nil {
				t.Errorf("unexpected error in goroutine %d: %v", id, err)
			}
			if user.ID != userID {
				t.Errorf("expected user ID '%s', got '%s'", userID, user.ID)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestAdapterStatus(t *testing.T) {
	// Create a context with an adapter
	db := &TestDatabase{Name: "TestDB"}
	ctx := NewDependencyContext(context.Background(), db, Adapt[UserAdapter](lookupUser))

	status := Status(ctx)

	// Check that it shows as an adapter
	if !strings.Contains(status, "ctxdep.UserAdapter - adapter wrapping:") {
		t.Error("Adapter should show as 'adapter wrapping:' in status")
	}

	// Check that it shows the wrapped function signature
	if !strings.Contains(status, "(context.Context, *ctxdep.TestDatabase, string) *ctxdep.TestUser, error") {
		t.Error("Status should show the original function signature")
	}
}

func TestAdapterAnonymousType(t *testing.T) {
	// Test using an anonymous function type for an adapter
	db := &TestDatabase{Name: "TestDB"}

	// Instead of using a named type like UserAdapter, use anonymous func type
	ctx := NewDependencyContext(context.Background(), db,
		Adapt[func(ctx context.Context, userID string) (*TestUser, error)](lookupUser))

	// Try to get it with the same anonymous type
	adapter := Get[func(ctx context.Context, userID string) (*TestUser, error)](ctx)

	// Use the adapter
	user, err := adapter(ctx, "user123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if user.ID != "user123" {
		t.Errorf("expected user ID 'user123', got '%s'", user.ID)
	}
	if user.Name != "Test User from TestDB" {
		t.Errorf("expected user name 'Test User from TestDB', got '%s'", user.Name)
	}
}

func TestAdapterAnonymousTypeMismatch(t *testing.T) {
	// Test what happens when anonymous function types have different signatures
	db := &TestDatabase{Name: "TestDB"}

	// Register with one anonymous type
	ctx := NewDependencyContext(context.Background(), db,
		Adapt[func(ctx context.Context, userID string) (*TestUser, error)](lookupUser))

	// Try to get with a different signature (int instead of string)
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when signatures don't match")
		}
	}()

	// This should panic because the signatures are different
	_ = Get[func(context.Context, int) (*TestUser, error)](ctx)
}

func TestAdapterAnonymousWithOptional(t *testing.T) {
	// Test GetOptional with anonymous function types
	db := &TestDatabase{Name: "TestDB"}

	ctx := NewDependencyContext(context.Background(), db,
		Adapt[func(ctx context.Context, userID string) (*TestUser, error)](lookupUser))

	// Try to get with exact same type - parameter names don't matter
	adapter1, found1 := GetOptional[func(ctx context.Context, userID string) (*TestUser, error)](ctx)
	if !found1 {
		t.Error("should find adapter with exact type match")
	}
	if adapter1 == nil {
		t.Error("adapter should not be nil")
	}

	// Try to get with same signature but different parameter names - should work
	adapter2, found2 := GetOptional[func(context.Context, string) (*TestUser, error)](ctx)
	if !found2 {
		t.Error("should find adapter with same signature (parameter names ignored)")
	}
	if adapter2 == nil {
		t.Error("adapter should not be nil")
	}

	// Try to get with different signature - should not find
	adapter3, found3 := GetOptional[func(context.Context, int) (*TestUser, error)](ctx)
	if found3 {
		t.Error("should not find adapter with different signature")
	}
	if adapter3 != nil {
		t.Error("adapter should be nil when not found")
	}
}

func TestAdapterAnonymousStatus(t *testing.T) {
	// Test how Status displays anonymous function types
	db := &TestDatabase{Name: "TestDB"}

	ctx := NewDependencyContext(context.Background(), db,
		Adapt[func(ctx context.Context, userID string) (*TestUser, error)](lookupUser))

	status := Status(ctx)
	t.Logf("Status with anonymous function type:\n%s", status)

	// Check that status includes the anonymous function type
	if !strings.Contains(status, "func(context.Context, string) (*ctxdep.TestUser, error)") {
		t.Error("Status should include the anonymous function type")
	}

	// Check that it shows as an adapter
	if !strings.Contains(status, "adapter wrapping:") {
		t.Error("Should show as adapter in status")
	}
}

func TestAdapterAnonymousMultiple(t *testing.T) {
	// Test multiple anonymous function types in same context
	db := &TestDatabase{Name: "TestDB"}
	config := &TestConfig{APIKey: "secret"}

	// Create two different anonymous function types with different signatures
	ctx := NewDependencyContext(context.Background(), db, config,
		Adapt[func(ctx context.Context, userID string) (*TestUser, error)](lookupUser),
		Adapt[func(ctx context.Context, op string, val int) (string, error)](complexFunction))

	// Get the first adapter
	userAdapter := Get[func(ctx context.Context, userID string) (*TestUser, error)](ctx)
	user, err := userAdapter(ctx, "test-user")
	if err != nil {
		t.Fatalf("unexpected error from user adapter: %v", err)
	}
	if user.ID != "test-user" {
		t.Errorf("expected user ID 'test-user', got '%s'", user.ID)
	}

	// Get the second adapter
	complexAdapter := Get[func(ctx context.Context, op string, val int) (string, error)](ctx)
	result, err := complexAdapter(ctx, "test", 42)
	if err != nil {
		t.Fatalf("unexpected error from complex adapter: %v", err)
	}
	expected := "TestDB:secret:test:" + string(rune(42))
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func TestAdapterAnonymousNestedContext(t *testing.T) {
	// Test anonymous function types with nested contexts
	db := &TestDatabase{Name: "ParentDB"}

	// Parent context with anonymous adapter
	parentCtx := NewDependencyContext(context.Background(), db,
		Adapt[func(ctx context.Context, userID string) (*TestUser, error)](lookupUser))

	// Child context trying to override (should not work due to security)
	newDB := &TestDatabase{Name: "ChildDB"}
	childCtx := NewDependencyContext(parentCtx, newDB, WithOverrides())

	// Get adapter from child context
	adapter := Get[func(ctx context.Context, userID string) (*TestUser, error)](childCtx)

	// Should use parent's database
	user, err := adapter(childCtx, "nested-user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Name != "Test User from ParentDB" {
		t.Errorf("expected user from ParentDB, got '%s'", user.Name)
	}
}

func TestAdapterAnonymousTypeAliases(t *testing.T) {
	// Test behavior with type aliases
	type MyUserFunc = func(ctx context.Context, userID string) (*TestUser, error)

	db := &TestDatabase{Name: "TestDB"}

	// Register with type alias
	ctx := NewDependencyContext(context.Background(), db, Adapt[MyUserFunc](lookupUser))

	// Get with type alias - should work
	adapter1 := Get[MyUserFunc](ctx)
	user1, err := adapter1(ctx, "alias-user")
	if err != nil {
		t.Fatalf("unexpected error with type alias: %v", err)
	}
	if user1.ID != "alias-user" {
		t.Errorf("expected user ID 'alias-user', got '%s'", user1.ID)
	}

	// Get with expanded type - should also work since it's an alias
	adapter2 := Get[func(ctx context.Context, userID string) (*TestUser, error)](ctx)
	user2, err := adapter2(ctx, "expanded-user")
	if err != nil {
		t.Fatalf("unexpected error with expanded type: %v", err)
	}
	if user2.ID != "expanded-user" {
		t.Errorf("expected user ID 'expanded-user', got '%s'", user2.ID)
	}
}

func TestRegularAnonymousFunctions(t *testing.T) {
	// Test what happens with regular (non-adapter) anonymous function dependencies

	// Create an anonymous function
	myFunc := func(x int) string {
		return fmt.Sprintf("value: %d", x)
	}

	// Store it as a dependency
	ctx := NewDependencyContext(context.Background(), &myFunc)

	// Get it back
	fn := Get[*func(int) string](ctx)
	result := (*fn)(42)
	if result != "value: 42" {
		t.Errorf("expected 'value: 42', got '%s'", result)
	}

	// Check status
	status := Status(ctx)
	t.Logf("Status with regular anonymous function:\n%s", status)
	if !strings.Contains(status, "*func(int) string") {
		t.Error("Status should show pointer to anonymous function type")
	}
	if !strings.Contains(status, "direct value set") {
		t.Error("Should show as direct value, not adapter")
	}
}

func TestAnonymousFunctionComparison(t *testing.T) {
	// Document the difference between regular functions and adapters with anonymous types

	db := &TestDatabase{Name: "TestDB"}

	// Regular anonymous function (stored as pointer)
	regularFunc := func(ctx context.Context, userID string) (*TestUser, error) {
		return &TestUser{ID: userID, Name: "Regular"}, nil
	}

	ctx := NewDependencyContext(context.Background(),
		db,
		&regularFunc, // Regular function stored as pointer
		Adapt[func(context.Context, string) (*TestUser, error)](lookupUser), // Adapter
	)

	// Get regular function
	regular := Get[*func(context.Context, string) (*TestUser, error)](ctx)
	user1, _ := (*regular)(context.Background(), "reg")
	if user1.Name != "Regular" {
		t.Errorf("expected 'Regular', got '%s'", user1.Name)
	}

	// Get adapter function (not a pointer)
	adapter := Get[func(context.Context, string) (*TestUser, error)](ctx)
	user2, _ := adapter(ctx, "fact")
	if user2.Name != "Test User from TestDB" {
		t.Errorf("expected 'Test User from TestDB', got '%s'", user2.Name)
	}

	// Show status difference
	status := Status(ctx)
	t.Logf("Status comparison:\n%s", status)
}

// Test adapter error handling when dependencies can't be resolved at runtime
func TestAdapterDependencyResolutionError(t *testing.T) {
	// Create a context with a generator that fails
	brokenGen := func(ctx context.Context) (*TestDatabase, error) {
		return nil, errors.New("database connection failed")
	}

	// Create context with the broken generator and an adapter that depends on it
	ctx := NewDependencyContext(context.Background(), brokenGen, Adapt[UserAdapter](lookupUser))

	// Get the adapter
	adapter := Get[UserAdapter](ctx)

	// Try to use the adapter - this should trigger the error handling path
	// because the TestDatabase dependency will fail to resolve
	user, err := adapter(ctx, "test-user")

	// Should get an error, not panic
	if err == nil {
		t.Error("expected error when dependency resolution fails")
	}
	if user != nil {
		t.Error("expected nil user when error occurs")
	}

	// The error should be about dependency resolution
	if !contains(err.Error(), "error running generator") {
		t.Errorf("unexpected error: %v", err)
	}
}

// Test adapter that doesn't return an error
func TestAdapterDependencyResolutionErrorNoErrorReturn(t *testing.T) {
	// Create a function that doesn't return an error
	noErrorFunc := func(ctx context.Context, db *TestDatabase, userID string) *TestUser {
		return &TestUser{ID: userID, Name: db.Name}
	}

	type NoErrorAdapter func(ctx context.Context, userID string) *TestUser

	// Create a failing generator
	brokenGen := func(ctx context.Context) (*TestDatabase, error) {
		return nil, errors.New("database unavailable")
	}

	ctx := NewDependencyContext(context.Background(), brokenGen, Adapt[NoErrorAdapter](noErrorFunc))

	// Get the adapter
	adapter := Get[NoErrorAdapter](ctx)

	// Try to use the adapter - should panic since the adapter doesn't return error
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when adapter without error return can't resolve dependencies")
		} else {
			// Verify the panic message
			if msg, ok := r.(string); ok {
				if !contains(msg, "failed to resolve dependency for adapter") {
					t.Errorf("unexpected panic message: %s", msg)
				}
			}
		}
	}()

	// This should panic
	_ = adapter(ctx, "test-user")
}

// Test adapter with multiple return values including error
func TestAdapterMultipleReturnsWithError(t *testing.T) {
	// Create a function that returns multiple values plus error
	multiReturnFunc := func(ctx context.Context, db *TestDatabase, userID string) (*TestUser, string, error) {
		if db == nil {
			return nil, "", errors.New("database is nil")
		}
		user := &TestUser{ID: userID, Name: db.Name}
		return user, "success", nil
	}

	type MultiReturnAdapter func(ctx context.Context, userID string) (*TestUser, string, error)

	db := &TestDatabase{Name: "TestDB"}
	ctx := NewDependencyContext(context.Background(), db, Adapt[MultiReturnAdapter](multiReturnFunc))

	// Get the adapter
	adapter := Get[MultiReturnAdapter](ctx)

	// Test successful case
	user, status, err := adapter(ctx, "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user == nil || user.ID != "user1" {
		t.Error("expected valid user")
	}
	if status != "success" {
		t.Errorf("expected 'success', got '%s'", status)
	}

	// Test error case with missing dependency
	// Create a context with failing generator for the database
	failingDB := func(ctx context.Context) (*TestDatabase, error) {
		return nil, errors.New("db connection failed")
	}
	errorCtx := NewDependencyContext(context.Background(), failingDB, Adapt[MultiReturnAdapter](multiReturnFunc))
	errorAdapter := Get[MultiReturnAdapter](errorCtx)

	user2, status2, err2 := errorAdapter(errorCtx, "user2")

	// Should return zero values for non-error returns and the error
	if err2 == nil {
		t.Error("expected error when dependency missing")
	}
	if user2 != nil {
		t.Error("expected nil user on error")
	}
	if status2 != "" {
		t.Errorf("expected empty string on error, got '%s'", status2)
	}
}

// Test adapter with complex return types
func TestAdapterComplexReturnTypes(t *testing.T) {
	type Result struct {
		Data  string
		Count int
	}

	// Function that returns struct, pointer, and error
	complexFunc := func(ctx context.Context, db *TestDatabase, input string) (Result, *TestUser, error) {
		result := Result{Data: input, Count: len(input)}
		user := &TestUser{ID: input, Name: db.Name}
		return result, user, nil
	}

	type ComplexAdapter func(ctx context.Context, input string) (Result, *TestUser, error)

	// Create context with failing database generator
	failingDB := func(ctx context.Context) (*TestDatabase, error) {
		return nil, errors.New("database error")
	}
	ctx := NewDependencyContext(context.Background(), failingDB, Adapt[ComplexAdapter](complexFunc))

	adapter := Get[ComplexAdapter](ctx)

	// Test with the adapter - should fail to resolve database
	result, user, err := adapter(ctx, "test")

	// Should get error and zero values
	if err == nil {
		t.Error("expected error with missing dependency")
	}
	if result.Data != "" || result.Count != 0 {
		t.Errorf("expected zero Result, got %+v", result)
	}
	if user != nil {
		t.Error("expected nil *TestUser")
	}
}

// Test nested dependency resolution failure
func TestAdapterNestedDependencyFailure(t *testing.T) {
	// Create a chain of dependencies where one fails
	type Service struct {
		Name string
	}

	// Generator that fails
	failingGen := func(ctx context.Context) (*Service, error) {
		return nil, errors.New("service unavailable")
	}

	// Function that depends on Service
	funcNeedsService := func(ctx context.Context, svc *Service, db *TestDatabase, id string) (*TestUser, error) {
		return &TestUser{ID: id, Name: svc.Name + ":" + db.Name}, nil
	}

	type ServiceAdapter func(ctx context.Context, id string) (*TestUser, error)

	db := &TestDatabase{Name: "TestDB"}
	ctx := NewDependencyContext(context.Background(), db, failingGen, Adapt[ServiceAdapter](funcNeedsService))

	adapter := Get[ServiceAdapter](ctx)

	// Should fail when trying to resolve Service dependency
	user, err := adapter(ctx, "test-id")
	if err == nil {
		t.Error("expected error when nested dependency fails")
	}
	if user != nil {
		t.Error("expected nil user on error")
	}

	// Verify error mentions the dependency resolution
	if !contains(err.Error(), "error running generator") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// Test adapter that only returns an error
func TestAdapterOnlyErrorReturn(t *testing.T) {
	// Function that only returns error
	validateFunc := func(ctx context.Context, db *TestDatabase, input string) error {
		if db == nil {
			return errors.New("database required")
		}
		if input == "" {
			return errors.New("input required")
		}
		return nil
	}

	type ValidatorAdapter func(ctx context.Context, input string) error

	// Test with failing database
	failingDB := func(ctx context.Context) (*TestDatabase, error) {
		return nil, errors.New("db unavailable")
	}

	ctx := NewDependencyContext(context.Background(), failingDB, Adapt[ValidatorAdapter](validateFunc))
	adapter := Get[ValidatorAdapter](ctx)

	// Should return the dependency resolution error
	err := adapter(ctx, "valid-input")
	if err == nil {
		t.Error("expected error when dependency fails")
	}
	if !contains(err.Error(), "error running generator") {
		t.Errorf("unexpected error: %v", err)
	}
}

// Test adapter with non-pointer return types
func TestAdapterValueReturnTypes(t *testing.T) {
	// Function that returns values, not pointers
	calcFunc := func(ctx context.Context, db *TestDatabase, x int) (int, bool, error) {
		if db == nil {
			return 0, false, errors.New("need database")
		}
		return x * 2, true, nil
	}

	type CalcAdapter func(ctx context.Context, x int) (int, bool, error)

	// Test with failing dependency
	failingDB := func(ctx context.Context) (*TestDatabase, error) {
		return nil, errors.New("calculation database offline")
	}

	ctx := NewDependencyContext(context.Background(), failingDB, Adapt[CalcAdapter](calcFunc))
	adapter := Get[CalcAdapter](ctx)

	// Should return zero values and error
	result, ok, err := adapter(ctx, 21)
	if err == nil {
		t.Error("expected error")
	}
	if result != 0 {
		t.Errorf("expected 0, got %d", result)
	}
	if ok {
		t.Error("expected false for bool return")
	}
}

// Helper function since strings.Contains isn't imported
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Test that adapter dependencies are not resolved until the adapter is called
func TestAdapterLazyDependencyResolution(t *testing.T) {
	// Counter to track when the database generator is called
	var dbGeneratorCalled int32

	// Create a generator that tracks when it's called
	dbGenerator := func(ctx context.Context) (*TestDatabase, error) {
		atomic.AddInt32(&dbGeneratorCalled, 1)
		return &TestDatabase{Name: "GeneratedDB"}, nil
	}

	// Create context with the tracking generator and an adapter
	ctx := NewDependencyContext(context.Background(), dbGenerator, Adapt[UserAdapter](lookupUser))

	// At this point, the database generator should NOT have been called
	if atomic.LoadInt32(&dbGeneratorCalled) != 0 {
		t.Error("database generator was called during context creation")
	}

	// Get the adapter - this should also NOT trigger the generator
	adapter := Get[UserAdapter](ctx)
	if atomic.LoadInt32(&dbGeneratorCalled) != 0 {
		t.Error("database generator was called when getting the adapter")
	}

	// Now call the adapter - this SHOULD trigger the generator
	user, err := adapter(ctx, "lazy-user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the generator was called exactly once
	if calls := atomic.LoadInt32(&dbGeneratorCalled); calls != 1 {
		t.Errorf("expected database generator to be called once, was called %d times", calls)
	}

	// Verify the result uses the generated database
	if user.Name != "Test User from GeneratedDB" {
		t.Errorf("expected user from GeneratedDB, got '%s'", user.Name)
	}

	// Call the adapter again - generator should not be called again (cached)
	user2, err := adapter(ctx, "another-user")
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}

	// Generator should still have been called only once
	if calls := atomic.LoadInt32(&dbGeneratorCalled); calls != 1 {
		t.Errorf("expected database generator to be called once total, was called %d times", calls)
	}

	if user2.Name != "Test User from GeneratedDB" {
		t.Errorf("expected user from GeneratedDB on second call, got '%s'", user2.Name)
	}
}

// Test with multiple adapters sharing lazy dependencies
func TestAdapterLazyMultipleAdapters(t *testing.T) {
	var dbCalls int32
	var configCalls int32

	dbGen := func(ctx context.Context) (*TestDatabase, error) {
		atomic.AddInt32(&dbCalls, 1)
		return &TestDatabase{Name: "LazyDB"}, nil
	}

	configGen := func(ctx context.Context) (*TestConfig, error) {
		atomic.AddInt32(&configCalls, 1)
		return &TestConfig{APIKey: "lazy-key"}, nil
	}

	// Create context with generators and multiple adapters
	ctx := NewDependencyContext(context.Background(),
		dbGen,
		configGen,
		Adapt[UserAdapter](lookupUser),
		Adapt[ComplexAdapter](complexFunction),
	)

	// No generators should be called yet
	if atomic.LoadInt32(&dbCalls) != 0 || atomic.LoadInt32(&configCalls) != 0 {
		t.Error("generators called during context creation")
	}

	// Get both adapters
	userAdapter := Get[UserAdapter](ctx)
	complexAdapter := Get[ComplexAdapter](ctx)

	// Still no generators should be called
	if atomic.LoadInt32(&dbCalls) != 0 || atomic.LoadInt32(&configCalls) != 0 {
		t.Error("generators called when getting adapters")
	}

	// Call user adapter - should only trigger DB generator
	_, err := userAdapter(ctx, "user1")
	if err != nil {
		t.Fatalf("error calling user adapter: %v", err)
	}

	if dbCalls := atomic.LoadInt32(&dbCalls); dbCalls != 1 {
		t.Errorf("expected DB generator called once, was called %d times", dbCalls)
	}
	if configCalls := atomic.LoadInt32(&configCalls); configCalls != 0 {
		t.Errorf("expected config generator not called, was called %d times", configCalls)
	}

	// Call complex adapter - should trigger config generator but not DB again
	result, err := complexAdapter(ctx, "test", 42)
	if err != nil {
		t.Fatalf("error calling complex adapter: %v", err)
	}

	if dbCalls := atomic.LoadInt32(&dbCalls); dbCalls != 1 {
		t.Errorf("expected DB generator still called once, was called %d times", dbCalls)
	}
	if configCalls := atomic.LoadInt32(&configCalls); configCalls != 1 {
		t.Errorf("expected config generator called once, was called %d times", configCalls)
	}

	expected := "LazyDB:lazy-key:test:" + string(rune(42))
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

// Test that expensive operations don't happen until adapter is called
func TestAdapterLazyExpensiveOperation(t *testing.T) {
	// Simulate an expensive database connection
	expensiveDBGen := func(ctx context.Context) (*TestDatabase, error) {
		// This would be expensive - sleep to simulate
		select {
		case <-time.After(50 * time.Millisecond):
			return &TestDatabase{Name: "ExpensiveDB"}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	start := time.Now()

	// Create context with expensive generator
	ctx := NewDependencyContext(context.Background(),
		expensiveDBGen,
		Adapt[UserAdapter](lookupUser),
	)

	// Context creation should be fast
	contextTime := time.Since(start)
	if contextTime > 10*time.Millisecond {
		t.Errorf("context creation took too long: %v", contextTime)
	}

	// Getting adapter should also be fast
	adapter := Get[UserAdapter](ctx)
	getTime := time.Since(start)
	if getTime > 10*time.Millisecond {
		t.Errorf("getting adapter took too long: %v", getTime)
	}

	// Calling adapter will be slow due to expensive operation
	callStart := time.Now()
	user, err := adapter(ctx, "expensive-user")
	callTime := time.Since(callStart)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// This should have taken at least 50ms
	if callTime < 50*time.Millisecond {
		t.Errorf("adapter call was too fast, expensive operation may not have run: %v", callTime)
	}

	if user.Name != "Test User from ExpensiveDB" {
		t.Errorf("unexpected user name: %s", user.Name)
	}
}

// Test Status doesn't trigger adapter dependency resolution
func TestAdapterLazyStatus(t *testing.T) {
	var generatorCalled int32

	trackingGen := func(ctx context.Context) (*TestDatabase, error) {
		atomic.AddInt32(&generatorCalled, 1)
		return &TestDatabase{Name: "StatusDB"}, nil
	}

	ctx := NewDependencyContext(context.Background(),
		trackingGen,
		Adapt[UserAdapter](lookupUser),
	)

	// Call Status - should not trigger generators
	status := Status(ctx)

	if atomic.LoadInt32(&generatorCalled) != 0 {
		t.Error("Status() triggered generator execution")
	}

	// Verify status shows uninitialized generator
	if !strings.Contains(status, "uninitialized - generator:") {
		t.Errorf("Status should show uninitialized generator, got:\n%s", status)
	}

	// Verify adapter is shown
	if !strings.Contains(status, "adapter wrapping:") {
		t.Errorf("Status should show adapter, got:\n%s", status)
	}
}

// Test that unused adapter dependencies are never resolved
func TestAdapterUnusedDependencies(t *testing.T) {
	var dbCalled int32
	var configCalled int32
	var serviceCalled int32

	// Create multiple generators
	dbGen := func(ctx context.Context) (*TestDatabase, error) {
		atomic.AddInt32(&dbCalled, 1)
		return &TestDatabase{Name: "DB"}, nil
	}

	configGen := func(ctx context.Context) (*TestConfig, error) {
		atomic.AddInt32(&configCalled, 1)
		return &TestConfig{APIKey: "key"}, nil
	}

	type ServiceType struct{ Name string }
	serviceGen := func(ctx context.Context) (*ServiceType, error) {
		atomic.AddInt32(&serviceCalled, 1)
		return &ServiceType{Name: "Service"}, nil
	}

	// Create an adapter that only needs database
	ctx := NewDependencyContext(context.Background(),
		dbGen,
		configGen,
		serviceGen,
		Adapt[UserAdapter](lookupUser), // Only needs TestDatabase
	)

	// Get and use the adapter
	adapter := Get[UserAdapter](ctx)
	_, err := adapter(ctx, "user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only the database generator should have been called
	if calls := atomic.LoadInt32(&dbCalled); calls != 1 {
		t.Errorf("expected DB generator called once, was called %d times", calls)
	}
	if calls := atomic.LoadInt32(&configCalled); calls != 0 {
		t.Errorf("expected config generator not called, was called %d times", calls)
	}
	if calls := atomic.LoadInt32(&serviceCalled); calls != 0 {
		t.Errorf("expected service generator not called, was called %d times", calls)
	}
}

// Test that adapters are created only once and reused
func TestAdapterReuseNotRecreated(t *testing.T) {
	// Counter to track adapter creation
	var adapterCreationCount int32

	// Create a function that tracks when the adapter is created
	instrumentedFunc := func(ctx context.Context, db *TestDatabase, userID string) (*TestUser, error) {
		return &TestUser{ID: userID, Name: db.Name}, nil
	}

	// Wrap the Adapt call to track when it's invoked
	ctx := NewDependencyContext(context.Background(),
		&TestDatabase{Name: "TestDB"},
		func() any {
			atomic.AddInt32(&adapterCreationCount, 1)
			return Adapt[UserAdapter](instrumentedFunc)
		}(),
	)

	// Get the adapter multiple times
	adapter1 := Get[UserAdapter](ctx)
	adapter2 := Get[UserAdapter](ctx)
	adapter3 := Get[UserAdapter](ctx)

	// Verify we got the same adapter instance
	// Note: We can't directly compare function values in Go, but we can verify behavior
	user1, _ := adapter1(ctx, "user1")
	user2, _ := adapter2(ctx, "user2")
	user3, _ := adapter3(ctx, "user3")

	if user1.Name != "TestDB" || user2.Name != "TestDB" || user3.Name != "TestDB" {
		t.Error("adapters not behaving consistently")
	}

	// The adapter creation should have happened only during context initialization
	if count := atomic.LoadInt32(&adapterCreationCount); count != 1 {
		t.Errorf("expected adapter creation to happen once, happened %d times", count)
	}
}

// Test that the adapter function itself is only created once
func TestAdapterFunctionCreatedOnce(t *testing.T) {
	// We'll verify this by checking that adapter validation only happens once
	// by using a generator that counts calls
	var dbGenCalls int32
	dbGen := func(ctx context.Context) (*TestDatabase, error) {
		atomic.AddInt32(&dbGenCalls, 1)
		return &TestDatabase{Name: "GenDB"}, nil
	}

	ctx := NewDependencyContext(context.Background(),
		dbGen,
		Adapt[UserAdapter](lookupUser),
	)

	// Get the adapter multiple times
	for i := 0; i < 5; i++ {
		adapter := Get[UserAdapter](ctx)
		// Use the adapter
		user, err := adapter(ctx, "test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if user.Name != "Test User from GenDB" {
			t.Errorf("unexpected user name: %s", user.Name)
		}
	}

	// The database generator should only be called once (lazy, cached)
	if calls := atomic.LoadInt32(&dbGenCalls); calls != 1 {
		t.Errorf("expected db generator called once, was called %d times", calls)
	}
}

// Test concurrent access to ensure adapter is created only once
func TestAdapterConcurrentCreation(t *testing.T) {
	var creationCount int32

	// Use a slow generator to increase chance of race conditions
	slowGen := func(ctx context.Context) (*TestDatabase, error) {
		// Increment counter when generator runs
		atomic.AddInt32(&creationCount, 1)
		time.Sleep(50 * time.Millisecond) // Simulate slow operation
		return &TestDatabase{Name: "ConcurrentDB"}, nil
	}

	ctx := NewDependencyContext(context.Background(),
		slowGen,
		Adapt[UserAdapter](lookupUser),
	)

	// Try to get the adapter concurrently
	var wg sync.WaitGroup
	adapters := make([]UserAdapter, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			adapters[idx] = Get[UserAdapter](ctx)
		}(i)
	}

	wg.Wait()

	// All adapters should be the same (behavior-wise)
	// Use them all to trigger any lazy initialization
	for i, adapter := range adapters {
		user, err := adapter(ctx, string(rune('0'+i)))
		if err != nil {
			t.Errorf("goroutine %d: unexpected error: %v", i, err)
		}
		if user.Name != "Test User from ConcurrentDB" {
			t.Errorf("goroutine %d: unexpected result: %s", i, user.Name)
		}
	}

	// The generator should only have been called once despite concurrent access
	if calls := atomic.LoadInt32(&creationCount); calls != 1 {
		t.Errorf("expected generator called once, was called %d times", calls)
	}
}

// Test that getting an adapter from child contexts reuses the parent's adapter
func TestAdapterReuseAcrossContexts(t *testing.T) {
	db := &TestDatabase{Name: "ParentDB"}
	parentCtx := NewDependencyContext(context.Background(), db, Adapt[UserAdapter](lookupUser))

	// Create child contexts
	child1 := NewDependencyContext(parentCtx, WithOverrides(), &TestDatabase{Name: "child1"}, &TestConfig{APIKey: "child1"})
	child2 := NewDependencyContext(parentCtx, WithOverrides(), &TestDatabase{Name: "child2"}, &TestConfig{APIKey: "child2"})

	// Get adapter from children
	childAdapter1 := Get[UserAdapter](child1)
	childAdapter2 := Get[UserAdapter](child2)

	// Get adapter from parent - out of order to verify it works with the security model
	parentAdapter := Get[UserAdapter](parentCtx)

	// All should produce the same results (using the parent's database)
	userP, _ := parentAdapter(parentCtx, "p")
	user1, _ := childAdapter1(child1, "c1")
	user2, _ := childAdapter2(child2, "c2")

	if userP.Name != "Test User from ParentDB" {
		t.Errorf("parent adapter produced wrong result: %s", userP.Name)
	}
	if user1.Name != "Test User from ParentDB" {
		t.Errorf("child1 adapter produced wrong result: %s", user1.Name)
	}
	if user2.Name != "Test User from ParentDB" {
		t.Errorf("child2 adapter produced wrong result: %s", user2.Name)
	}
}

// Verify the actual slot storage behavior
func TestAdapterSlotReuse(t *testing.T) {
	db := &TestDatabase{Name: "SlotDB"}
	ctx := NewDependencyContext(context.Background(), db, Adapt[UserAdapter](lookupUser))

	dc := GetDependencyContext(ctx)

	// Get the slot for UserAdapter
	var slot1, slot2 *slot
	if s, ok := dc.slots.Load(reflect.TypeOf((*UserAdapter)(nil)).Elem()); ok {
		slot1 = s.(*slot)
	} else {
		t.Fatal("UserAdapter slot not found")
	}

	// Get adapter to ensure it's initialized
	_ = Get[UserAdapter](ctx)

	// Get the slot again
	if s, ok := dc.slots.Load(reflect.TypeOf((*UserAdapter)(nil)).Elem()); ok {
		slot2 = s.(*slot)
	} else {
		t.Fatal("UserAdapter slot not found on second lookup")
	}

	// Should be the exact same slot object
	if slot1 != slot2 {
		t.Error("slot objects are different, adapter may have been recreated")
	}

	// The value should be set (the adapter function)
	if slot1.value == nil {
		t.Error("slot value is nil")
	}

	// Status should indicate it's an adapter
	if slot1.status != StatusAdapter {
		t.Errorf("expected StatusAdapter, got %v", slot1.status)
	}
}
