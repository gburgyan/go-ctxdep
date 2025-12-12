# Design Decisions

This library makes several design decisions that may seem surprising at first, but each serves a specific purpose. Understanding these choices helps you use the library more effectively.

## Panic-First Error Handling

**What's surprising:** The library defaults to panicking when dependencies are not found.

```go
func Get[T any](ctx context.Context) T {
    // Panics if dependency not found
}
```

**Why this decision was made:**
- **Fail-fast philosophy**: Errors surface immediately rather than being hidden in production
- **Reduced boilerplate**: Eliminates the need for error checking everywhere in the call chain
- **Testing simplicity**: Makes unit tests cleaner since you know dependencies will be available
- **Explicit dependency declaration**: Forces developers to be explicit about what dependencies their code needs

The library provides `GetWithError()` and `GetOptional()` for cases where you want error handling, but the default behavior is intentionally aggressive.

## Dual Context Architecture

**What's surprising:** The library creates a `secureContext` wrapper that separates timing context from dependency context.

```go
type secureContext struct {
    baseContext   context.Context  // Contains dependencies
    timingContext context.Context  // Contains timing/deadlines
}
```

**Why this decision was made:**
- **Security isolation**: Prevents child contexts from accidentally accessing parent dependencies through timing context
- **Prevents data pollution**: Ensures that generator results from one context can't leak into another
- **Clean separation of concerns**: Timing/deadlines come from the caller's context, but dependencies come from the creation context
- **Prevents infinite loops**: Prevents potential security vulnerabilities where wrong data could be used

## Immediate Dependencies with Goroutines

**What's surprising:** The `Immediate()` feature starts generators in background goroutines and blocks access until completion.

```go
func (d *DependencyContext) resolveImmediateDependencies(ctx context.Context) {
    go func() {
        // Generator execution in background
    }()
}
```

**Why this decision was made:**
- **Performance optimization**: Allows expensive operations to start early while the main execution continues
- **Head start for expensive operations**: Particularly useful for database queries or external API calls
- **Non-blocking initialization**: The main execution path doesn't wait for immediate dependencies
- **Graceful error handling**: If immediate dependencies fail, they're retried when actually accessed

## Type-Based Dependency Resolution with Reflection

**What's surprising:** Heavy use of reflection for type matching and interface assignment.

```go
func canAssign(concrete, iface reflect.Type) bool {
    // Complex reflection-based interface matching
}
```

**Why this decision was made:**
- **Interface satisfaction**: Allows concrete types to satisfy interfaces automatically
- **Type safety**: Maintains compile-time type checking while providing runtime flexibility
- **Performance optimization**: Extensive caching of reflection results to minimize overhead
- **Flexible dependency resolution**: Supports complex dependency graphs without explicit wiring

## Context Locking and Overrideable Dependencies

**What's surprising:** The library has a sophisticated locking system that prevents dependency overrides in production while allowing specific overrides.

```go
func WithLock() ContextOption {
    // Locks context to prevent overrides
}

func Overrideable(dep any) any {
    // Marks specific dependencies as overrideable even in locked contexts
}
```

**Why this decision was made:**
- **Production safety**: Prevents accidental dependency overrides in production code
- **Testing flexibility**: Allows controlled overrides for testing scenarios
- **Logger pattern support**: Specifically designed to support Go's `slog` pattern where loggers are replaced with `With()` calls
- **Fine-grained control**: Different dependencies can have different override policies

## Adapter Pattern with Partial Application

**What's surprising:** The library implements a sophisticated adapter system that creates partially applied functions.

```go
type UserAdapter func(ctx context.Context, userID string) (*User, error)

func lookupUser(ctx context.Context, db *Database, userID string) (*User, error) {
    // Implementation
}

// Creates a function that has db injected but userID provided at call time
ctx := NewDependencyContext(ctx, db, Adapt[UserAdapter](lookupUser))
```

**Why this decision was made:**
- **Testing simplicity**: Eliminates need for mocking frameworks
- **Clean separation**: Business logic can be tested without database dependencies
- **Type safety**: Compile-time validation of adapter signatures
- **Performance**: Adapters are created once and reused, not recreated on each call

## Explicit Cleanup with Manual Control

**What's surprising:** Cleanup is not automatic and must be explicitly enabled and called.

```go
func WithCleanup() ContextOption {
    // Enables cleanup but doesn't make it automatic
}

func (dc *DependencyContext) Cleanup() {
    // Must be called explicitly
}
```

**Why this decision was made:**
- **Race condition prevention**: Avoids cleanup happening during concurrent access
- **Explicit control**: Gives developers full control over when resources are released
- **Predictable behavior**: No surprises about when cleanup occurs
- **Performance**: Avoids overhead of automatic cleanup mechanisms

## Cyclic Dependency Detection

**What's surprising:** Sophisticated cycle detection that prevents deadlocks.

```go
type cycleChecker struct {
    inProcess map[reflect.Type]bool
    lock      sync.Mutex
}
```

**Why this decision was made:**
- **Deadlock prevention**: Without this, circular dependencies would cause infinite loops
- **Early error detection**: Catches configuration errors at runtime with clear messages
- **Thread safety**: Ensures cycle detection works correctly in concurrent scenarios
- **Debugging support**: Provides clear error messages about cyclic dependencies

## Generator Validation at Context Creation

**What's surprising:** All generator dependencies are validated when the context is created, not when generators are called.

**Why this decision was made:**
- **Fail-fast**: Catches configuration errors immediately
- **No production surprises**: A rarely-used code path won't suddenly fail because a dependency is missing
- **Clear error location**: Errors point to context creation, not deep in the call stack

## Overall Design Philosophy

The library follows several key principles:

1. **Explicit over implicit**: Dependencies must be explicitly declared and managed
2. **Fail fast**: Errors are surfaced immediately rather than hidden
3. **Type safety**: Heavy use of generics and reflection while maintaining compile-time safety
4. **Performance conscious**: Extensive caching and optimization of reflection operations
5. **Testing first**: Many features (like adapters and overrides) are designed specifically for testing scenarios
6. **Security minded**: Prevents data leakage between contexts and ensures proper isolation

The most surprising aspect is how the library balances simplicity with sophistication - it provides a simple interface while implementing complex machinery underneath to handle edge cases, performance, and safety concerns.

---

## See Also

- [Core Concepts](core-concepts.md) - How the library works
- [Advanced](advanced.md) - Technical details
