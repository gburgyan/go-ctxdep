# Core Concepts

This guide explains the fundamental concepts behind go-ctxdep.

## The Problem Being Solved

Go already has a nice way to keep track of things with `context.Context`. This library adds helpers to simplify getting things out of that context that is already being passed around.

The core motivation is **testing** - making it easy to override dependencies in tests without global state or complex mocking frameworks. See the [Testing Guide](testing.md) for the full story.

## Dependency Context Hierarchy

Dependency contexts can be nested, similar to how `context.WithValue()` works:

```go
// Service-level dependencies
serviceCtx := ctxdep.NewDependencyContext(ctx, database, logger)

// Request-level dependencies (inherits from service level)
requestCtx := ctxdep.NewDependencyContext(serviceCtx, &Request{ID: 123})

// Can access both service and request dependencies
db := ctxdep.Get[*Database](requestCtx)
req := ctxdep.Get[*Request](requestCtx)
```

When looking for a dependency:
1. The current dependency context is checked first
2. If not found, parent dependency contexts are searched
3. If still not found, an error occurs (panic by default)

**Important:** A lower-level context (e.g., service) cannot depend on a higher-level context (e.g., request). This is enforced at context creation time to prevent caching issues.

## Types of Dependencies

### Direct Values

The simplest form - just pass pointers to structs:

```go
ctx := ctxdep.NewDependencyContext(ctx, &MyService{}, &Config{})
```

### Generators

Functions that create dependencies on first access:

```go
ctx := ctxdep.NewDependencyContext(ctx, func(ctx context.Context) (*UserData, error) {
    return fetchUserData(ctx)
})
```

See [Generators](generators.md) for comprehensive coverage.

### Adapters

Partial function application - some parameters from context, some at call time:

```go
ctx := ctxdep.NewDependencyContext(ctx, db, ctxdep.Adapt[UserLookup](lookupUser))
```

See [Adapters](adapters.md) for details.

## Interface Resolution

When a concrete type implements an interface, you can request either:

```go
ctx := ctxdep.NewDependencyContext(ctx, &ServiceCaller{})

// Get as concrete type
impl := ctxdep.Get[*ServiceCaller](ctx)

// Or get as interface (automatic cast)
svc := ctxdep.Get[Service](ctx)
```

**Edge case:** If multiple types implement the same interface, which one is returned is undefined. Avoid this situation by being explicit about which type you want.

## Fail-Fast Philosophy

The library defaults to panicking when dependencies are not found:

```go
// Panics if *UserData is not in context
user := ctxdep.Get[*UserData](ctx)
```

**Why panic?**
- Errors surface immediately rather than being hidden
- Eliminates error checking boilerplate everywhere
- Forces developers to be explicit about dependencies
- Makes testing simpler

### When You Want Error Handling

For cases where you need to handle errors:

```go
// Returns error instead of panicking
user, err := ctxdep.GetWithError[*UserData](ctx)
if err != nil {
    // Handle missing dependency
}
```

The error will be of type `ctxdep.DependencyError` with the context status included for debugging.

### Checking for Dependency Context

If you need to check whether a dependency context exists at all (useful in library code):

```go
dc, err := ctxdep.GetDependencyContextWithError(ctx)
if err != nil {
    // No dependency context - handle accordingly
}
```

Note that `GetOptional` handles this case automatically and returns `(zero, false)` when no dependency context exists.

### Optional Dependencies

For truly optional dependencies:

```go
// Returns (value, found) - never panics for missing deps
cache, found := ctxdep.GetOptional[*CacheService](ctx)
if found {
    // Use cache
}
```

See [Optional Dependencies](optional-deps.md) for more patterns.

## Getting Multiple Values

If you need multiple values at once:

```go
var widget *Widget
var doodad *Doodad
ctxdep.GetBatch(ctx, &widget, &doodad)
```

This is a slight optimization - it only looks up the dependency context once.

Variants:
- `GetBatch(ctx, ptrs...)` - panics on missing
- `GetBatchWithError(ctx, ptrs...)` - returns error
- `GetBatchOptional(ctx, ptrs...)` - returns `[]bool` indicating which were found

## Dependency Validation at Creation

When adding generators, the context validates that all dependencies can be fulfilled:

```go
// This panics immediately because *Database is not available
ctx := ctxdep.NewDependencyContext(ctx,
    func(db *Database) *UserService {
        return &UserService{db: db}
    },
)
```

This fail-fast behavior prevents runtime errors in production when a rarely-used code path triggers a missing dependency.

---

## See Also

- [Generators](generators.md) - Lazy dependency creation
- [Testing](testing.md) - Why this matters for testing
- [Optional Dependencies](optional-deps.md) - Graceful handling of missing dependencies
