# Context Control

This guide covers locking, overrides, and other mechanisms for controlling context behavior.

## Strict vs Loose Construction

By default, adding duplicate dependencies causes a panic:

```go
// This panics - two values fill the same slot
ctx := ctxdep.NewDependencyContext(ctx, widgetA, widgetB)
```

This fail-fast behavior catches configuration errors early.

### WithOverrides() for Testing

For tests, use `WithOverrides()` to allow overriding:

```go
// This works - widgetB overrides widgetA
ctx := ctxdep.NewDependencyContext(ctx,
    ctxdep.WithOverrides(),
    widgetA,
    widgetB,
)
```

Options and dependencies can be mixed:

```go
ctx := ctxdep.NewDependencyContext(ctx,
    widgetA,
    ctxdep.WithOverrides(),
    widgetB,
)
```

**Behavior with overrides:**
- Last value wins for concrete types
- Last generator wins for generator types
- A concrete value always wins over a generator

## Context Locking

In production, you may want to prevent accidental overriding.

### WithLock() at Creation

```go
prodCtx := ctxdep.NewDependencyContext(ctx,
    ctxdep.WithLock(),  // Lock this context
    database,
    logger,
    config,
)

// This panics - cannot use WithOverrides() on locked parent
testCtx := ctxdep.NewDependencyContext(prodCtx, ctxdep.WithOverrides(), mockDB)

// This is fine - adding new dependencies without overrides
childCtx := ctxdep.NewDependencyContext(prodCtx, userService)
```

### Lock() After Creation

Lock a context after creation - useful when the same code runs in tests and production:

```go
func CreateAppContext() *ctxdep.DependencyContext {
    return ctxdep.NewDependencyContext(context.Background(),
        database,
        logger,
        config,
    )
}

// In tests - leave unlocked for flexibility
testCtx := CreateAppContext()
mockCtx := ctxdep.NewDependencyContext(testCtx, ctxdep.WithOverrides(), mockDB)

// In production - lock after creation
prodCtx := CreateAppContext()
prodCtx.Lock()  // or ctxdep.Lock(prodCtx)
// Now overrides will panic
```

Once locked, a context cannot be unlocked.

## Overrideable Dependencies

Sometimes specific dependencies (like loggers) should be replaceable even in locked contexts.

### The slog Logger Pattern

Go's `slog` package uses a pattern where loggers are replaced with `With()`:

```go
logger := slog.Default()
requestLogger := logger.With("request_id", requestID, "user_id", userID)
```

With `Overrideable()`, you can support this pattern:

```go
ctx := ctxdep.NewDependencyContext(ctx,
    ctxdep.WithLock(),                     // Lock the context
    ctxdep.Overrideable(slog.Default()),   // But logger can be overridden
    database,                              // Database cannot be overridden
)

// This works - logger is overrideable
requestLogger := logger.With("request_id", requestID)
childCtx := ctxdep.NewDependencyContext(ctx, requestLogger)

// This panics - database is not overrideable in locked context
errorCtx := ctxdep.NewDependencyContext(ctx, mockDatabase)
```

### Key Points

1. **Declaration timing**: Dependencies must be marked overrideable when first introduced
2. **Works with generators**: Both direct dependencies and generators can be overrideable
3. **Inheritance**: Overrideable status is checked up the entire context chain
4. **Common use cases**:
   - Loggers with `With()` patterns
   - Feature flags
   - Metrics collectors disabled in tests

## Overriding the Parent Context

In some cases, you need to override where parent dependencies are looked up. This is useful when:
- gRPC services have `context.Background()` on service goroutines
- The calling context doesn't have the dependencies you need

Pass a `context.Context` as the first dependency:

```go
// serviceCtx has the dependencies we need
// but grpcCtx is context.Background()
dc := ctxdep.NewDependencyContext(grpcCtx,
    serviceCtx,  // First context param overrides parent lookup
    requestDeps,
)
```

This works even if the context is inside a slice.

## Multiple Options

Options can be combined:

```go
ctx := ctxdep.NewDependencyContext(ctx,
    ctxdep.WithOverrides(),
    ctxdep.WithCleanup(),
    ctxdep.WithLock(),
    widgetA,
    widgetB,
)
```

## Deprecated: NewLooseDependencyContext

For backward compatibility, `NewLooseDependencyContext` still works but is deprecated:

```go
// Old way (deprecated)
ctx := ctxdep.NewLooseDependencyContext(ctx, widgetA, widgetB)

// New way
ctx := ctxdep.NewDependencyContext(ctx, ctxdep.WithOverrides(), widgetA, widgetB)
```

---

## See Also

- [Testing](testing.md) - Using WithOverrides() in tests
- [Design Decisions](design-decisions.md) - Why locking works this way
