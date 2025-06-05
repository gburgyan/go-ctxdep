package ctxdep

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Test types for factory tests
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

// Factory function types
type UserFactory func(ctx context.Context, userID string) (*TestUser, error)
type UserFactoryNoError func(ctx context.Context, userID string) *TestUser
type ComplexFactory func(ctx context.Context, name string, age int) (string, error)

// Test functions that will be wrapped as factories
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

func TestFactoryBasic(t *testing.T) {
	// Create a context with dependencies
	db := &TestDatabase{Name: "TestDB"}
	ctx := NewDependencyContext(context.Background(), db, Factory[UserFactory](lookupUser))

	// Get the factory
	factory := Get[UserFactory](ctx)
	if factory == nil {
		t.Fatal("factory should not be nil")
	}

	// Use the factory
	user, err := factory(ctx, "user123")
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

func TestFactoryWithError(t *testing.T) {
	// Create a context with dependencies
	db := &TestDatabase{Name: "TestDB"}
	ctx := NewDependencyContext(context.Background(), db, Factory[UserFactory](lookupUser))

	// Get the factory
	factory := Get[UserFactory](ctx)

	// Use the factory with error case
	user, err := factory(ctx, "error")
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

func TestFactoryNoError(t *testing.T) {
	// Create a context with dependencies
	db := &TestDatabase{Name: "TestDB"}
	ctx := NewDependencyContext(context.Background(), db, Factory[UserFactoryNoError](lookupUserNoError))

	// Get the factory
	factory := Get[UserFactoryNoError](ctx)

	// Use the factory
	user := factory(ctx, "user456")
	if user == nil {
		t.Fatal("user should not be nil")
	}
	if user.ID != "user456" {
		t.Errorf("expected user ID 'user456', got '%s'", user.ID)
	}
}

func TestFactoryMultipleDependencies(t *testing.T) {
	// Create a context with multiple dependencies
	db := &TestDatabase{Name: "TestDB"}
	config := &TestConfig{APIKey: "secret123"}
	ctx := NewDependencyContext(context.Background(), db, config, Factory[ComplexFactory](complexFunction))

	// Get the factory
	factory := Get[ComplexFactory](ctx)

	// Use the factory
	result, err := factory(ctx, "John", 30)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "TestDB:secret123:John:" + string(rune(30))
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func TestFactoryMissingDependency(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for missing dependency")
		}
	}()

	// Create a context without the required database dependency
	_ = NewDependencyContext(context.Background(), Factory[UserFactory](lookupUser))
	// This should panic during initialization
}

func TestFactoryInvalidTargetType(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid target type")
		}
	}()

	// Try to create a factory with non-function target type
	type NotAFunction struct{}
	Factory[NotAFunction](lookupUser)
}

func TestFactoryInvalidFunction(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid function")
		}
	}()

	// Try to create a factory with non-function argument
	Factory[UserFactory]("not a function")
}

func TestFactoryParameterMismatch(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for parameter mismatch")
		}
	}()

	// Wrong factory type - expects different parameters
	type WrongFactory func(ctx context.Context, userID string, extra int) (*TestUser, error)
	db := &TestDatabase{Name: "TestDB"}
	_ = NewDependencyContext(context.Background(), db, Factory[WrongFactory](lookupUser))
}

func TestFactoryReturnMismatch(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for return mismatch")
		}
	}()

	// Wrong factory type - expects different return types
	type WrongFactory func(ctx context.Context, userID string) (string, error)
	Factory[WrongFactory](lookupUser)
}

func TestFactoryWithOptionalDependency(t *testing.T) {
	// Test that factory validates missing dependencies at initialization
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when factory has unresolvable dependencies")
		}
	}()

	// Function that requires a dependency not in context
	fn := func(ctx context.Context, missing *TestConfig, id string) (*TestUser, error) {
		return &TestUser{ID: id}, nil
	}

	type TestFactory func(ctx context.Context, id string) (*TestUser, error)

	// This should panic because TestConfig is not available
	ctx := NewDependencyContext(context.Background(), Factory[TestFactory](fn))
	_ = ctx
}

func TestFactoryContextUpdate(t *testing.T) {
	// Test that factory uses dependencies from creation time, not from the provided context
	db := &TestDatabase{Name: "OriginalDB"}
	ctx := NewDependencyContext(context.Background(), db, Factory[UserFactory](lookupUser))

	factory := Get[UserFactory](ctx)

	// Create a new context with updated database
	newDB := &TestDatabase{Name: "UpdatedDB"}
	newCtx := NewDependencyContext(ctx, newDB, WithOverrides())

	// Use factory with new context
	user, err := factory(newCtx, "user789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should use the original database from when factory was created
	if user.Name != "Test User from OriginalDB" {
		t.Errorf("expected user from OriginalDB, got '%s'", user.Name)
	}
}

func TestFactoryNoContextParameter(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for factory without context parameter")
		}
	}()

	// Factory target requires context but original function doesn't have it
	noCtxFunc := func(db *TestDatabase, userID string) (*TestUser, error) {
		return &TestUser{ID: userID}, nil
	}

	db := &TestDatabase{Name: "TestDB"}
	ctx := NewDependencyContext(context.Background(), db, Factory[UserFactory](noCtxFunc))

	factory := Get[UserFactory](ctx)
	// This should panic when trying to call the factory
	factory(ctx, "test")
}

func TestFactoryAllDependenciesFromContext(t *testing.T) {
	// Test function where all non-context parameters come from context
	type SimpleFactory func(ctx context.Context) (*TestUser, error)

	fn := func(ctx context.Context, db *TestDatabase, config *TestConfig) (*TestUser, error) {
		return &TestUser{
			ID:    config.APIKey,
			Name:  db.Name,
			Email: "test@example.com",
		}, nil
	}

	db := &TestDatabase{Name: "TestDB"}
	config := &TestConfig{APIKey: "key123"}
	ctx := NewDependencyContext(context.Background(), db, config, Factory[SimpleFactory](fn))

	factory := Get[SimpleFactory](ctx)
	user, err := factory(ctx)
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

func TestFactoryConcurrent(t *testing.T) {
	// Test concurrent factory usage
	db := &TestDatabase{Name: "TestDB"}
	ctx := NewDependencyContext(context.Background(), db, Factory[UserFactory](lookupUser))

	factory := Get[UserFactory](ctx)

	// Run multiple goroutines using the factory
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(id int) {
			userID := "user" + string(rune('0'+id))
			user, err := factory(ctx, userID)
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

func TestFactoryStatus(t *testing.T) {
	// Create a context with a factory
	db := &TestDatabase{Name: "TestDB"}
	ctx := NewDependencyContext(context.Background(), db, Factory[UserFactory](lookupUser))

	status := Status(ctx)

	// Check that it shows as a factory
	if !strings.Contains(status, "ctxdep.UserFactory - factory wrapping:") {
		t.Error("Factory should show as 'factory wrapping:' in status")
	}

	// Check that it shows the wrapped function signature
	if !strings.Contains(status, "(context.Context, *ctxdep.TestDatabase, string) *ctxdep.TestUser, error") {
		t.Error("Status should show the original function signature")
	}
}

func TestFactoryAnonymousType(t *testing.T) {
	// Test using an anonymous function type for a factory
	db := &TestDatabase{Name: "TestDB"}

	// Instead of using a named type like UserFactory, use anonymous func type
	ctx := NewDependencyContext(context.Background(), db,
		Factory[func(ctx context.Context, userID string) (*TestUser, error)](lookupUser))

	// Try to get it with the same anonymous type
	factory := Get[func(ctx context.Context, userID string) (*TestUser, error)](ctx)

	// Use the factory
	user, err := factory(ctx, "user123")
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

func TestFactoryAnonymousTypeMismatch(t *testing.T) {
	// Test what happens when anonymous function types have different signatures
	db := &TestDatabase{Name: "TestDB"}

	// Register with one anonymous type
	ctx := NewDependencyContext(context.Background(), db,
		Factory[func(ctx context.Context, userID string) (*TestUser, error)](lookupUser))

	// Try to get with a different signature (int instead of string)
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when signatures don't match")
		}
	}()

	// This should panic because the signatures are different
	_ = Get[func(context.Context, int) (*TestUser, error)](ctx)
}

func TestFactoryAnonymousWithOptional(t *testing.T) {
	// Test GetOptional with anonymous function types
	db := &TestDatabase{Name: "TestDB"}

	ctx := NewDependencyContext(context.Background(), db,
		Factory[func(ctx context.Context, userID string) (*TestUser, error)](lookupUser))

	// Try to get with exact same type - parameter names don't matter
	factory1, found1 := GetOptional[func(ctx context.Context, userID string) (*TestUser, error)](ctx)
	if !found1 {
		t.Error("should find factory with exact type match")
	}
	if factory1 == nil {
		t.Error("factory should not be nil")
	}

	// Try to get with same signature but different parameter names - should work
	factory2, found2 := GetOptional[func(context.Context, string) (*TestUser, error)](ctx)
	if !found2 {
		t.Error("should find factory with same signature (parameter names ignored)")
	}
	if factory2 == nil {
		t.Error("factory should not be nil")
	}

	// Try to get with different signature - should not find
	factory3, found3 := GetOptional[func(context.Context, int) (*TestUser, error)](ctx)
	if found3 {
		t.Error("should not find factory with different signature")
	}
	if factory3 != nil {
		t.Error("factory should be nil when not found")
	}
}

func TestFactoryAnonymousStatus(t *testing.T) {
	// Test how Status displays anonymous function types
	db := &TestDatabase{Name: "TestDB"}

	ctx := NewDependencyContext(context.Background(), db,
		Factory[func(ctx context.Context, userID string) (*TestUser, error)](lookupUser))

	status := Status(ctx)
	t.Logf("Status with anonymous function type:\n%s", status)

	// Check that status includes the anonymous function type
	if !strings.Contains(status, "func(context.Context, string) (*ctxdep.TestUser, error)") {
		t.Error("Status should include the anonymous function type")
	}

	// Check that it shows as a factory
	if !strings.Contains(status, "factory wrapping:") {
		t.Error("Should show as factory in status")
	}
}

func TestFactoryAnonymousMultiple(t *testing.T) {
	// Test multiple anonymous function types in same context
	db := &TestDatabase{Name: "TestDB"}
	config := &TestConfig{APIKey: "secret"}

	// Create two different anonymous function types with different signatures
	ctx := NewDependencyContext(context.Background(), db, config,
		Factory[func(ctx context.Context, userID string) (*TestUser, error)](lookupUser),
		Factory[func(ctx context.Context, op string, val int) (string, error)](complexFunction))

	// Get the first factory
	userFactory := Get[func(ctx context.Context, userID string) (*TestUser, error)](ctx)
	user, err := userFactory(ctx, "test-user")
	if err != nil {
		t.Fatalf("unexpected error from user factory: %v", err)
	}
	if user.ID != "test-user" {
		t.Errorf("expected user ID 'test-user', got '%s'", user.ID)
	}

	// Get the second factory
	complexFactory := Get[func(ctx context.Context, op string, val int) (string, error)](ctx)
	result, err := complexFactory(ctx, "test", 42)
	if err != nil {
		t.Fatalf("unexpected error from complex factory: %v", err)
	}
	expected := "TestDB:secret:test:" + string(rune(42))
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

func TestFactoryAnonymousNestedContext(t *testing.T) {
	// Test anonymous function types with nested contexts
	db := &TestDatabase{Name: "ParentDB"}

	// Parent context with anonymous factory
	parentCtx := NewDependencyContext(context.Background(), db,
		Factory[func(ctx context.Context, userID string) (*TestUser, error)](lookupUser))

	// Child context trying to override (should not work due to security)
	newDB := &TestDatabase{Name: "ChildDB"}
	childCtx := NewDependencyContext(parentCtx, newDB, WithOverrides())

	// Get factory from child context
	factory := Get[func(ctx context.Context, userID string) (*TestUser, error)](childCtx)

	// Should use parent's database
	user, err := factory(childCtx, "nested-user")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.Name != "Test User from ParentDB" {
		t.Errorf("expected user from ParentDB, got '%s'", user.Name)
	}
}

func TestFactoryAnonymousTypeAliases(t *testing.T) {
	// Test behavior with type aliases
	type MyUserFunc = func(ctx context.Context, userID string) (*TestUser, error)

	db := &TestDatabase{Name: "TestDB"}

	// Register with type alias
	ctx := NewDependencyContext(context.Background(), db, Factory[MyUserFunc](lookupUser))

	// Get with type alias - should work
	factory1 := Get[MyUserFunc](ctx)
	user1, err := factory1(ctx, "alias-user")
	if err != nil {
		t.Fatalf("unexpected error with type alias: %v", err)
	}
	if user1.ID != "alias-user" {
		t.Errorf("expected user ID 'alias-user', got '%s'", user1.ID)
	}

	// Get with expanded type - should also work since it's an alias
	factory2 := Get[func(ctx context.Context, userID string) (*TestUser, error)](ctx)
	user2, err := factory2(ctx, "expanded-user")
	if err != nil {
		t.Fatalf("unexpected error with expanded type: %v", err)
	}
	if user2.ID != "expanded-user" {
		t.Errorf("expected user ID 'expanded-user', got '%s'", user2.ID)
	}
}

func TestRegularAnonymousFunctions(t *testing.T) {
	// Test what happens with regular (non-factory) anonymous function dependencies

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
		t.Error("Should show as direct value, not factory")
	}
}

func TestAnonymousFunctionComparison(t *testing.T) {
	// Document the difference between regular functions and factories with anonymous types

	db := &TestDatabase{Name: "TestDB"}

	// Regular anonymous function (stored as pointer)
	regularFunc := func(ctx context.Context, userID string) (*TestUser, error) {
		return &TestUser{ID: userID, Name: "Regular"}, nil
	}

	ctx := NewDependencyContext(context.Background(),
		db,
		&regularFunc,                                                          // Regular function stored as pointer
		Factory[func(context.Context, string) (*TestUser, error)](lookupUser), // Factory
	)

	// Get regular function
	regular := Get[*func(context.Context, string) (*TestUser, error)](ctx)
	user1, _ := (*regular)(context.Background(), "reg")
	if user1.Name != "Regular" {
		t.Errorf("expected 'Regular', got '%s'", user1.Name)
	}

	// Get factory function (not a pointer)
	factory := Get[func(context.Context, string) (*TestUser, error)](ctx)
	user2, _ := factory(ctx, "fact")
	if user2.Name != "Test User from TestDB" {
		t.Errorf("expected 'Test User from TestDB', got '%s'", user2.Name)
	}

	// Show status difference
	status := Status(ctx)
	t.Logf("Status comparison:\n%s", status)
}

// Test factory error handling when dependencies can't be resolved at runtime
func TestFactoryDependencyResolutionError(t *testing.T) {
	// Create a context with a generator that fails
	brokenGen := func(ctx context.Context) (*TestDatabase, error) {
		return nil, errors.New("database connection failed")
	}

	// Create context with the broken generator and a factory that depends on it
	ctx := NewDependencyContext(context.Background(), brokenGen, Factory[UserFactory](lookupUser))

	// Get the factory
	factory := Get[UserFactory](ctx)

	// Try to use the factory - this should trigger the error handling path
	// because the TestDatabase dependency will fail to resolve
	user, err := factory(ctx, "test-user")

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

// Test factory that doesn't return an error
func TestFactoryDependencyResolutionErrorNoErrorReturn(t *testing.T) {
	// Create a function that doesn't return an error
	noErrorFunc := func(ctx context.Context, db *TestDatabase, userID string) *TestUser {
		return &TestUser{ID: userID, Name: db.Name}
	}

	type NoErrorFactory func(ctx context.Context, userID string) *TestUser

	// Create a failing generator
	brokenGen := func(ctx context.Context) (*TestDatabase, error) {
		return nil, errors.New("database unavailable")
	}

	ctx := NewDependencyContext(context.Background(), brokenGen, Factory[NoErrorFactory](noErrorFunc))

	// Get the factory
	factory := Get[NoErrorFactory](ctx)

	// Try to use the factory - should panic since the factory doesn't return error
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic when factory without error return can't resolve dependencies")
		} else {
			// Verify the panic message
			if msg, ok := r.(string); ok {
				if !contains(msg, "failed to resolve dependency for factory") {
					t.Errorf("unexpected panic message: %s", msg)
				}
			}
		}
	}()

	// This should panic
	_ = factory(ctx, "test-user")
}

// Test factory with multiple return values including error
func TestFactoryMultipleReturnsWithError(t *testing.T) {
	// Create a function that returns multiple values plus error
	multiReturnFunc := func(ctx context.Context, db *TestDatabase, userID string) (*TestUser, string, error) {
		if db == nil {
			return nil, "", errors.New("database is nil")
		}
		user := &TestUser{ID: userID, Name: db.Name}
		return user, "success", nil
	}

	type MultiReturnFactory func(ctx context.Context, userID string) (*TestUser, string, error)

	db := &TestDatabase{Name: "TestDB"}
	ctx := NewDependencyContext(context.Background(), db, Factory[MultiReturnFactory](multiReturnFunc))

	// Get the factory
	factory := Get[MultiReturnFactory](ctx)

	// Test successful case
	user, status, err := factory(ctx, "user1")
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
	errorCtx := NewDependencyContext(context.Background(), failingDB, Factory[MultiReturnFactory](multiReturnFunc))
	errorFactory := Get[MultiReturnFactory](errorCtx)

	user2, status2, err2 := errorFactory(errorCtx, "user2")

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

// Test factory with complex return types
func TestFactoryComplexReturnTypes(t *testing.T) {
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

	type ComplexFactory func(ctx context.Context, input string) (Result, *TestUser, error)

	// Create context with failing database generator
	failingDB := func(ctx context.Context) (*TestDatabase, error) {
		return nil, errors.New("database error")
	}
	ctx := NewDependencyContext(context.Background(), failingDB, Factory[ComplexFactory](complexFunc))

	factory := Get[ComplexFactory](ctx)

	// Test with the factory - should fail to resolve database
	result, user, err := factory(ctx, "test")

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
func TestFactoryNestedDependencyFailure(t *testing.T) {
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

	type ServiceFactory func(ctx context.Context, id string) (*TestUser, error)

	db := &TestDatabase{Name: "TestDB"}
	ctx := NewDependencyContext(context.Background(), db, failingGen, Factory[ServiceFactory](funcNeedsService))

	factory := Get[ServiceFactory](ctx)

	// Should fail when trying to resolve Service dependency
	user, err := factory(ctx, "test-id")
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

// Test factory that only returns an error
func TestFactoryOnlyErrorReturn(t *testing.T) {
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

	type ValidatorFactory func(ctx context.Context, input string) error

	// Test with failing database
	failingDB := func(ctx context.Context) (*TestDatabase, error) {
		return nil, errors.New("db unavailable")
	}

	ctx := NewDependencyContext(context.Background(), failingDB, Factory[ValidatorFactory](validateFunc))
	factory := Get[ValidatorFactory](ctx)

	// Should return the dependency resolution error
	err := factory(ctx, "valid-input")
	if err == nil {
		t.Error("expected error when dependency fails")
	}
	if !contains(err.Error(), "error running generator") {
		t.Errorf("unexpected error: %v", err)
	}
}

// Test factory with non-pointer return types
func TestFactoryValueReturnTypes(t *testing.T) {
	// Function that returns values, not pointers
	calcFunc := func(ctx context.Context, db *TestDatabase, x int) (int, bool, error) {
		if db == nil {
			return 0, false, errors.New("need database")
		}
		return x * 2, true, nil
	}

	type CalcFactory func(ctx context.Context, x int) (int, bool, error)

	// Test with failing dependency
	failingDB := func(ctx context.Context) (*TestDatabase, error) {
		return nil, errors.New("calculation database offline")
	}

	ctx := NewDependencyContext(context.Background(), failingDB, Factory[CalcFactory](calcFunc))
	factory := Get[CalcFactory](ctx)

	// Should return zero values and error
	result, ok, err := factory(ctx, 21)
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
// Test that factory dependencies are not resolved until the factory is called
func TestFactoryLazyDependencyResolution(t *testing.T) {
	// Counter to track when the database generator is called
	var dbGeneratorCalled int32
	
	// Create a generator that tracks when it's called
	dbGenerator := func(ctx context.Context) (*TestDatabase, error) {
		atomic.AddInt32(&dbGeneratorCalled, 1)
		return &TestDatabase{Name: "GeneratedDB"}, nil
	}
	
	// Create context with the tracking generator and a factory
	ctx := NewDependencyContext(context.Background(), dbGenerator, Factory[UserFactory](lookupUser))
	
	// At this point, the database generator should NOT have been called
	if atomic.LoadInt32(&dbGeneratorCalled) != 0 {
		t.Error("database generator was called during context creation")
	}
	
	// Get the factory - this should also NOT trigger the generator
	factory := Get[UserFactory](ctx)
	if atomic.LoadInt32(&dbGeneratorCalled) != 0 {
		t.Error("database generator was called when getting the factory")
	}
	
	// Now call the factory - this SHOULD trigger the generator
	user, err := factory(ctx, "lazy-user")
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
	
	// Call the factory again - generator should not be called again (cached)
	user2, err := factory(ctx, "another-user")
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

// Test with multiple factories sharing lazy dependencies
func TestFactoryLazyMultipleFactories(t *testing.T) {
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
	
	// Create context with generators and multiple factories
	ctx := NewDependencyContext(context.Background(), 
		dbGen, 
		configGen,
		Factory[UserFactory](lookupUser),
		Factory[ComplexFactory](complexFunction),
	)
	
	// No generators should be called yet
	if atomic.LoadInt32(&dbCalls) != 0 || atomic.LoadInt32(&configCalls) != 0 {
		t.Error("generators called during context creation")
	}
	
	// Get both factories
	userFactory := Get[UserFactory](ctx)
	complexFactory := Get[ComplexFactory](ctx)
	
	// Still no generators should be called
	if atomic.LoadInt32(&dbCalls) != 0 || atomic.LoadInt32(&configCalls) != 0 {
		t.Error("generators called when getting factories")
	}
	
	// Call user factory - should only trigger DB generator
	_, err := userFactory(ctx, "user1")
	if err != nil {
		t.Fatalf("error calling user factory: %v", err)
	}
	
	if dbCalls := atomic.LoadInt32(&dbCalls); dbCalls != 1 {
		t.Errorf("expected DB generator called once, was called %d times", dbCalls)
	}
	if configCalls := atomic.LoadInt32(&configCalls); configCalls != 0 {
		t.Errorf("expected config generator not called, was called %d times", configCalls)
	}
	
	// Call complex factory - should trigger config generator but not DB again
	result, err := complexFactory(ctx, "test", 42)
	if err != nil {
		t.Fatalf("error calling complex factory: %v", err)
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

// Test that expensive operations don't happen until factory is called
func TestFactoryLazyExpensiveOperation(t *testing.T) {
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
		Factory[UserFactory](lookupUser),
	)
	
	// Context creation should be fast
	contextTime := time.Since(start)
	if contextTime > 10*time.Millisecond {
		t.Errorf("context creation took too long: %v", contextTime)
	}
	
	// Getting factory should also be fast
	factory := Get[UserFactory](ctx)
	getTime := time.Since(start)
	if getTime > 10*time.Millisecond {
		t.Errorf("getting factory took too long: %v", getTime)
	}
	
	// Calling factory will be slow due to expensive operation
	callStart := time.Now()
	user, err := factory(ctx, "expensive-user")
	callTime := time.Since(callStart)
	
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	
	// This should have taken at least 50ms
	if callTime < 50*time.Millisecond {
		t.Errorf("factory call was too fast, expensive operation may not have run: %v", callTime)
	}
	
	if user.Name != "Test User from ExpensiveDB" {
		t.Errorf("unexpected user name: %s", user.Name)
	}
}

// Test Status doesn't trigger factory dependency resolution
func TestFactoryLazyStatus(t *testing.T) {
	var generatorCalled int32
	
	trackingGen := func(ctx context.Context) (*TestDatabase, error) {
		atomic.AddInt32(&generatorCalled, 1)
		return &TestDatabase{Name: "StatusDB"}, nil
	}
	
	ctx := NewDependencyContext(context.Background(),
		trackingGen,
		Factory[UserFactory](lookupUser),
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
	
	// Verify factory is shown
	if !strings.Contains(status, "factory wrapping:") {
		t.Errorf("Status should show factory, got:\n%s", status)
	}
}

// Test that unused factory dependencies are never resolved
func TestFactoryUnusedDependencies(t *testing.T) {
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
	
	// Create a factory that only needs database
	ctx := NewDependencyContext(context.Background(),
		dbGen,
		configGen,
		serviceGen,
		Factory[UserFactory](lookupUser), // Only needs TestDatabase
	)
	
	// Get and use the factory
	factory := Get[UserFactory](ctx)
	_, err := factory(ctx, "user")
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
