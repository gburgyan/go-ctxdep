# Adapters

Adapters provide partial function application - some parameters come from the dependency context, others are provided at call time. This is particularly powerful for testing.

## What Are Adapters?

An adapter wraps a function so that some of its parameters are filled from the context while others remain as the adapter's parameters:

```go
// Original function: needs db AND userID
func lookupUser(ctx context.Context, db Database, userID string) (*User, error) {
    return db.GetUser(ctx, userID)
}

// Adapter type: only needs userID (db comes from context)
type UserAdapter func(ctx context.Context, userID string) (*User, error)
```

When you create an adapter, the context dependencies (`db`) are captured. When you call the adapter, you provide only the remaining parameters (`userID`).

## Basic Usage

```go
type Database interface {
    GetUser(ctx context.Context, id string) (*User, error)
}

type User struct {
    ID    string
    Name  string
    Email string
}

// Define an adapter type
type UserAdapter func(ctx context.Context, userID string) (*User, error)

// Function that needs dependencies
func lookupUser(ctx context.Context, db Database, userID string) (*User, error) {
    return db.GetUser(ctx, userID)
}

func main() {
    db := connectToDatabase()

    // Register the adapter
    ctx := ctxdep.NewDependencyContext(context.Background(),
        db,
        ctxdep.Adapt[UserAdapter](lookupUser),
    )

    // Get and use the adapter
    adapter := ctxdep.Get[UserAdapter](ctx)
    user, err := adapter(ctx, "user123")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Found user: %+v\n", user)
}
```

## Adapters with Multiple Dependencies

Adapters can capture multiple dependencies:

```go
type Config struct {
    APIKey string
}

type ComplexAdapter func(ctx context.Context, operation string, value int) (string, error)

func complexOperation(ctx context.Context, db Database, cfg *Config, operation string, value int) (string, error) {
    user, _ := db.GetUser(ctx, cfg.APIKey)
    return fmt.Sprintf("%s: %s processed %d", operation, user.Name, value), nil
}

func main() {
    db := connectToDatabase()
    cfg := &Config{APIKey: "admin"}

    ctx := ctxdep.NewDependencyContext(context.Background(),
        db,
        cfg,
        ctxdep.Adapt[ComplexAdapter](complexOperation),
    )

    adapter := ctxdep.Get[ComplexAdapter](ctx)
    result, _ := adapter(ctx, "process", 42)
    fmt.Println(result)  // "process: Admin processed 42"
}
```

## Adapter Validation

Adapters are validated when the context is created:

```go
// This panics at context creation - Config is missing!
ctx := ctxdep.NewDependencyContext(context.Background(),
    db,  // Missing cfg!
    ctxdep.Adapt[ComplexAdapter](complexOperation),
)
```

Validation checks:
- All dependencies required by the original function can be resolved
- Parameter and return types match between adapter type and original function
- The adapter type includes `context.Context` if the original function has one

## Anonymous Function Types

You can use anonymous function types instead of named types:

```go
// Using anonymous function type
ctx := ctxdep.NewDependencyContext(context.Background(),
    db,
    ctxdep.Adapt[func(context.Context, string) (*User, error)](lookupUser),
)

// Retrieve with the same anonymous type
adapter := ctxdep.Get[func(context.Context, string) (*User, error)](ctx)
```

**Note:** Go considers anonymous function types identical based on their signature, not parameter names. These are the same type:
- `func(ctx context.Context, id string) (*User, error)`
- `func(context.Context, string) (*User, error)`

## Security Model

**Adapters capture dependencies from the context where they were created, not from the context passed when calling the adapter.**

This prevents child contexts from overriding dependencies that the adapter uses:

```go
// Parent context with real database
parentCtx := ctxdep.NewDependencyContext(ctx,
    realDB,
    ctxdep.Adapt[UserAdapter](lookupUser),
)

// Child context tries to override database
childCtx := ctxdep.NewDependencyContext(parentCtx,
    mockDB,  // This won't affect the adapter!
)

// Adapter still uses realDB, not mockDB
adapter := ctxdep.Get[UserAdapter](childCtx)
user, _ := adapter(childCtx, "123")  // Uses realDB
```

This is intentional - it prevents security vulnerabilities where malicious code could inject fake dependencies.

## Adapters vs Regular Function Dependencies

Regular functions can be stored as dependencies, but they don't provide partial application:

```go
// Regular function stored as dependency (pointer)
regularFunc := func(id string) *User {
    return &User{ID: id}
}
ctx := ctxdep.NewDependencyContext(ctx, &regularFunc)

// Retrieved as pointer
fn := ctxdep.Get[*func(string) *User](ctx)
user := (*fn)("123")  // Note: no context dependencies used!

// Adapter - provides partial application
ctx := ctxdep.NewDependencyContext(ctx,
    db,
    ctxdep.Adapt[func(context.Context, string) (*User, error)](lookupUser),
)
adapter := ctxdep.Get[func(context.Context, string) (*User, error)](ctx)
user, err := adapter(ctx, "123")  // db is injected from context
```

**When to use which:**
- **Regular functions**: When the function doesn't need context dependencies
- **Adapters**: When you want dependencies injected, especially for testing

## Adapters Cannot Be Generators

Adapters cannot be used as generators for other dependencies:

```go
// This is an adapter - returns a function
ctx := ctxdep.NewDependencyContext(ctx,
    db,
    ctxdep.Adapt[UserAdapter](lookupUser),
)

// Get the adapter, not a *User
adapter := ctxdep.Get[UserAdapter](ctx)

// You must call the adapter to get a User
user, _ := adapter(ctx, "123")
```

If you want a generated `*User`, use a regular generator:

```go
ctx := ctxdep.NewDependencyContext(ctx,
    db,
    func(ctx context.Context, db Database) (*User, error) {
        return db.GetUser(ctx, "default-user")
    },
)
user := ctxdep.Get[*User](ctx)  // This works
```

---

## See Also

- [Testing](testing.md) - Using adapters for clean testing
- [Generators](generators.md) - When to use generators vs adapters
