# Optional Dependencies

This guide covers two related features:
- **GetOptional()**: Retrieving dependencies that might not exist
- **Optional()**: Adding dependencies that might be nil

## Retrieving Optional Dependencies

Use `GetOptional()` when a dependency might not be present and you want to handle that gracefully:

```go
func processWithOptionalCache(ctx context.Context) {
    cache, found := ctxdep.GetOptional[*CacheService](ctx)
    if found {
        // Use cache for faster processing
        if result := cache.Get("key"); result != nil {
            return result
        }
    }
    // Fall back to slower processing without cache
    return computeExpensiveResult()
}
```

**Key difference from Get():**
- `Get[T](ctx)` panics if dependency not found
- `GetOptional[T](ctx)` returns `(value, false)` if not found
- `GetOptional[T](ctx)` also returns `(zero, false)` if no dependency context exists at all

### Use Cases

- **Feature flags**: Optional services that may be disabled
- **Graceful degradation**: Fallback when services are unavailable
- **Testing**: Run tests with and without certain dependencies
- **Library code**: Code that may or may not be called within a ctxdep-enabled application

### GetBatchOptional

For multiple optional dependencies:

```go
var cache *CacheService
var logger *LogService
var metrics *MetricsService

results := ctxdep.GetBatchOptional(ctx, &cache, &logger, &metrics)
// results[0] indicates if cache was found
// results[1] indicates if logger was found
// results[2] indicates if metrics was found

if results[0] {
    // Use cache
}
```

## Adding Optional Dependencies

Use `Optional()` when adding a dependency that might be nil. This is primarily useful in testing.

### The Problem

Normally, passing a typed nil pointer causes a panic:

```go
var mockDB *MockDatabase = nil
ctx := ctxdep.NewDependencyContext(ctx, mockDB)  // PANICS!
```

### The Solution

With `Optional()`, nil values are silently skipped:

```go
var mockDB *MockDatabase = nil
ctx := ctxdep.NewDependencyContext(ctx, ctxdep.Optional(mockDB))
// No panic - mockDB is simply not added to the context
```

If the value is non-nil, it's added normally:

```go
mockDB := &MockDatabase{...}
ctx := ctxdep.NewDependencyContext(ctx, ctxdep.Optional(mockDB))
// mockDB is added normally
db := ctxdep.Get[*MockDatabase](ctx)  // Works!
```

### Testing Pattern

A common pattern is test helpers that accept optional mock dependencies:

```go
func setupTestContext(mockDB *MockDatabase, mockCache *CacheService) context.Context {
    return ctxdep.NewDependencyContext(context.Background(),
        realLogger,
        ctxdep.Optional(mockDB),    // Use mock if provided, otherwise skip
        ctxdep.Optional(mockCache), // Use mock if provided, otherwise skip
    )
}

func TestWithMockDB(t *testing.T) {
    mockDB := &MockDatabase{...}
    ctx := setupTestContext(mockDB, nil)  // Only mock the DB
    // Test code...
}

func TestWithBothMocks(t *testing.T) {
    mockDB := &MockDatabase{...}
    mockCache := &CacheService{...}
    ctx := setupTestContext(mockDB, mockCache)  // Mock both
    // Test code...
}
```

## Constraints

`Optional()` has several safety constraints:

### Only Pointer and Interface Types

Non-pointer types cannot be nil, so they're not allowed:

```go
ctxdep.Optional(42)           // PANICS - int is not a pointer
ctxdep.Optional("hello")      // PANICS - string is not a pointer
ctxdep.Optional(&MyStruct{})  // OK - pointer type
```

### Generators Cannot Be Optional

Generators provide types that other dependencies might depend on. Making them optional would create unpredictable behavior:

```go
gen := func() *MyType { return &MyType{} }
ctxdep.Optional(gen)  // PANICS - generators not allowed
```

### Cannot Combine with Other Wrappers

For clarity and safety, `Optional()` cannot be nested with other wrappers:

```go
ctxdep.Optional(ctxdep.Immediate(gen))      // PANICS
ctxdep.Immediate(ctxdep.Optional(dep))      // PANICS
ctxdep.Optional(ctxdep.Overrideable(dep))   // PANICS
ctxdep.Overrideable(ctxdep.Optional(dep))   // PANICS
```

### Dependent Generator Validation

If an optional dependency is nil and skipped, any generator that depends on it will fail validation:

```go
var nilDep *MyDep = nil
gen := func(dep *MyDep) *Result { return &Result{} }

// This fails validation because gen needs *MyDep which wasn't added
ctx, err := ctxdep.NewDependencyContextWithValidation(ctx,
    ctxdep.Optional(nilDep),
    gen,
)
// err: "generator has dependencies that cannot be resolved"
```

This is the expected behavior - if the optional dependency was needed, don't pass nil.

---

## See Also

- [Core Concepts](core-concepts.md) - GetWithError() and error handling
- [Testing](testing.md) - Using Optional() in test helpers
