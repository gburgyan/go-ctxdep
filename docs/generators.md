# Generators

Generators are the most powerful feature of go-ctxdep. They allow lazy creation of dependencies - the dependency is only created when first requested, and the result is cached for subsequent requests.

## What is a Generator?

A generator is simply a function that returns one or more types. When you add a generator to a dependency context, the return types become available as dependencies. The function is called the first time any of those types are requested.

```go
ctx := ctxdep.NewDependencyContext(ctx, func() *Widget {
    return &Widget{Name: "created lazily"}
})

// Generator is called here, on first access
widget := ctxdep.Get[*Widget](ctx)

// Returns cached result - generator is NOT called again
widget2 := ctxdep.Get[*Widget](ctx)
```

## Why Use Generators?

Generators solve several common problems:

1. **Expensive operations**: Database lookups, API calls, file reads
2. **Request-specific data**: User data, product info tied to a request
3. **Conditional creation**: Only create if actually needed
4. **Dependency chaining**: One dependency needs another to be created

## Transparent to Consumers

A key insight: **consumers don't know or care whether a dependency came from a generator or a direct value.** They just call `Get[T](ctx)` and receive the value.

This is powerful for testing. In production, you might have a complex generator:

```go
// Production: generator does expensive database lookup
func UserDataGenerator(ctx context.Context, db *Database, req *Request) (*UserData, error) {
    return db.LookupUser(ctx, req.UserID)
}

func HandleRequest(ctx context.Context, request *Request) {
    ctx = ctxdep.NewDependencyContext(ctx,
        database,
        request,
        UserDataGenerator,  // Complex generator
    )
    processRequest(ctx)
}
```

But in tests, you skip the generator entirely and provide the result directly:

```go
func TestProcessRequest(t *testing.T) {
    // Test: just provide the value directly - no generator, no database
    ctx := ctxdep.NewDependencyContext(context.Background(),
        &UserData{ID: 123, Name: "Test User", IsAdmin: true},
    )

    processRequest(ctx)  // Works exactly the same!
}
```

The `processRequest` function has no idea whether `*UserData` came from a generator that hit a database, or was just placed directly in the context. This means:

- **Test what matters**: Test your business logic without invoking expensive generators
- **No mocking required**: You don't mock the generator - you just don't use it
- **Fast tests**: No database calls, no API calls, no file I/O
- **Isolated tests**: Each test provides exactly the data it needs

See the [Testing Guide](testing.md) for comprehensive patterns.

## Generator Signatures

Generators support several function signatures:

### Simple Generator

```go
func() *Widget
```

Returns a single type, no parameters needed.

### Context-Aware Generator

```go
func(ctx context.Context) (*Widget, error)
```

Receives the calling context (for timeouts/cancellation) and can return errors.

### Generator with Dependencies

```go
func(ctx context.Context, db *Database, config *Config) (*Widget, error)
```

Parameters are automatically resolved from the context before the generator is called.

### Multi-Output Generator

```go
func() (*Widget, *Doodad)
```

Returns multiple types - all are registered as dependencies from a single generator call.

## Chained Dependencies

The most powerful pattern is when generators depend on other dependencies:

```go
type UserDataService interface {
    Lookup(request *Request) *UserData
}

type UserData struct {
    ID      int
    Name    string
    IsAdmin bool
}

func UserDataGenerator(ctx context.Context, svc UserDataService, req *Request) (*UserData, error) {
    return svc.Lookup(req), nil
}

func HandleRequest(ctx context.Context, request *Request) Response {
    ctx = ctxdep.NewDependencyContext(ctx,
        &UserDataServiceImpl{},
        request,
        UserDataGenerator,
    )
    return isPermitted(ctx)
}

func isPermitted(ctx context.Context) bool {
    user := ctxdep.Get[*UserData](ctx)  // Triggers UserDataGenerator
    return user.IsAdmin
}
```

When `*UserData` is requested:
1. The context sees `UserDataGenerator` provides `*UserData`
2. It resolves `UserDataService` and `*Request` from the context
3. Calls `UserDataGenerator` with those dependencies
4. Caches and returns the result

### The Simplification Journey

The above can be written in several equivalent ways. Here's the progression:

**Verbose (factory function returning closure):**
```go
func UserDataGenerator(request *Request) func(context.Context) (*UserData, error) {
    return func(ctx context.Context) (*UserData, error) {
        userService := ctxdep.Get[UserDataService](ctx)
        return userService.Lookup(request), nil
    }
}
```

**Simpler (request from context):**
```go
func UserDataGenerator(ctx context.Context) (*UserData, error) {
    userService := ctxdep.Get[UserDataService](ctx)
    request := ctxdep.Get[*Request](ctx)
    return userService.Lookup(request), nil
}
```

**Simplest (dependencies as parameters):**
```go
func UserDataGenerator(ctx context.Context, svc UserDataService, req *Request) (*UserData, error) {
    return svc.Lookup(req), nil
}
```

The last form is recommended - it's clearer and the dependency context handles resolution automatically.

## Immediate Generators

Sometimes you know a dependency is expensive AND always needed. Use `Immediate()` to start generation in a background goroutine:

```go
ctx := ctxdep.NewDependencyContext(ctx,
    &UserDataServiceImpl{},
    request,
    ctxdep.Immediate(UserDataGenerator),  // Starts running NOW
)

// Do other work while UserDataGenerator runs in background...
doOtherSetup()

// This might block briefly if generator isn't done yet
user := ctxdep.Get[*UserData](ctx)
```

**How it works:**
1. `Immediate()` starts the generator in a new goroutine immediately
2. The main execution continues without waiting
3. When the result is accessed, it either returns immediately (if done) or blocks until ready

**When to use Immediate:**
- Database connections that are always needed
- External API calls at the start of request handling
- Any expensive operation where you want a head start

**Important notes:**
- If the immediate generator fails, the error is logged and the generator is retried on access
- Errors from immediate generators are not cached - subsequent accesses will retry

## Multi-Output Generators

A single generator can provide multiple types:

```go
func createServices() (*UserService, *AuthService) {
    shared := &sharedState{}
    return &UserService{state: shared}, &AuthService{state: shared}
}

ctx := ctxdep.NewDependencyContext(ctx, createServices)

// Both come from the same generator call
userSvc := ctxdep.Get[*UserService](ctx)
authSvc := ctxdep.Get[*AuthService](ctx)
```

The generator is called once; both results are cached. This is useful when:
- Multiple types share initialization logic
- Types need to share internal state
- Creation order matters

## Error Handling in Generators

Generators can return errors as their last return value:

```go
func loadConfig(ctx context.Context) (*Config, error) {
    data, err := os.ReadFile("config.json")
    if err != nil {
        return nil, fmt.Errorf("loading config: %w", err)
    }
    var cfg Config
    if err := json.Unmarshal(data, &cfg); err != nil {
        return nil, fmt.Errorf("parsing config: %w", err)
    }
    return &cfg, nil
}
```

**Error behavior:**
- With `Get[T](ctx)`: Generator errors cause a panic
- With `GetWithError[T](ctx)`: Returns the error for handling
- **Errors are NOT cached** - subsequent access attempts will re-run the generator

```go
// Using GetWithError for graceful handling
cfg, err := ctxdep.GetWithError[*Config](ctx)
if err != nil {
    log.Printf("Config load failed: %v", err)
    cfg = &Config{Defaults: true}
}
```

## Dependency Validation

When you add a generator, the context validates that all its dependencies can be resolved:

```go
// This panics at context creation time!
ctx := ctxdep.NewDependencyContext(ctx,
    func(db *Database) *UserService {  // *Database not in context
        return &UserService{db: db}
    },
)
```

This fail-fast behavior catches configuration errors early, not when some rare code path happens to need the dependency in production.

The validation considers:
- Direct values in the current context
- Generators' return types in the current context
- Dependencies available from parent contexts

## Best Practices

### When to Use Generators vs Direct Values

**Use direct values when:**
- The object is already created (e.g., request, config)
- Creation is trivial (e.g., empty struct)
- You want explicit control over initialization order

**Use generators when:**
- Creation is expensive (DB queries, API calls)
- The dependency might not be needed
- The dependency needs other dependencies to be created

### Generator Design Patterns

**Pattern: Factory generators for request-specific closures**
```go
func OrderValidatorGenerator(ctx context.Context, db *Database) func(orderID string) error {
    return func(orderID string) error {
        return db.ValidateOrder(ctx, orderID)
    }
}
```

**Pattern: Conditional initialization**
```go
func CacheGenerator(ctx context.Context, cfg *Config) (*Cache, error) {
    if !cfg.CacheEnabled {
        return nil, nil  // OK to return nil if that's valid
    }
    return cache.New(cfg.CacheConfig)
}
```

### Common Pitfalls

1. **Circular dependencies**: Generator A needs B, but B needs A. Detected at runtime with a clear error.

2. **Nil returns**: Returning nil from a generator is allowed but may cause issues downstream. Consider using `Optional()` patterns.

3. **Context cancellation**: Generators receive the caller's context. Respect timeouts and cancellation.

4. **Side effects**: Generators are cached after first call. Don't rely on them being called multiple times.

---

## See Also

- [Caching](caching.md) - Cache generator results across requests
- [Testing](testing.md) - Mock generators in tests
- [Advanced](advanced.md) - Thread safety and timing integration
