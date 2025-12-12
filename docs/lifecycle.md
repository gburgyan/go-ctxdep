# Lifecycle Management

Dependencies can have cleanup functions that are called when explicitly requested. This is useful for resources like database connections, file handles, or network connections.

## Enabling Cleanup

Cleanup is opt-in and must be explicitly enabled:

### WithCleanup()

Enables automatic detection of `io.Closer`:

```go
type DatabaseConnection struct {
    conn *sql.DB
}

func (dc *DatabaseConnection) Close() error {
    return dc.conn.Close()
}

func main() {
    ctx := context.Background()

    dbConn := &DatabaseConnection{conn: openDB()}
    dc := ctxdep.NewDependencyContext(ctx,
        ctxdep.WithCleanup(),  // Enable cleanup functionality
        dbConn,
    )
    defer dc.Cleanup()  // Call cleanup when function returns

    // Use the database connection
    // When the function returns, dbConn.Close() will be called
}
```

### WithCleanupFunc()

For custom cleanup logic or types that don't implement `io.Closer`:

```go
type Service struct {
    workers []*Worker
}

func cleanupService(s *Service) {
    for _, worker := range s.workers {
        worker.Stop()
    }
    log.Println("Service cleaned up")
}

func main() {
    ctx := context.Background()

    service := &Service{workers: startWorkers()}
    dc := ctxdep.NewDependencyContext(ctx,
        ctxdep.WithCleanupFunc(cleanupService),
        service,
    )
    defer dc.Cleanup()

    // When the function returns, cleanupService will be invoked
}
```

**Note:** Using `WithCleanupFunc()` automatically enables cleanup for all dependencies.

## io.Closer Integration

Any dependency implementing `io.Closer` will have `Close()` called during cleanup:

```go
type MyResource struct{}

func (r *MyResource) Close() error {
    fmt.Println("Resource closed")
    return nil
}

dc := ctxdep.NewDependencyContext(ctx,
    ctxdep.WithCleanup(),
    &MyResource{},
)
defer dc.Cleanup()  // Prints "Resource closed"
```

## Cleanup Behavior

### Explicit Invocation

Cleanup must be explicitly triggered:

```go
dc := ctxdep.NewDependencyContext(ctx, ctxdep.WithCleanup(), resource)
defer dc.Cleanup()  // Typical usage with defer
```

### Once-Only Execution

Cleanup functions run exactly once, even if `Cleanup()` is called multiple times:

```go
dc := ctxdep.NewDependencyContext(ctx, ctxdep.WithCleanup(), resource)
dc.Cleanup()  // Cleans up
dc.Cleanup()  // Does nothing - already cleaned
```

### Context Isolation

Each context manages its own dependencies. Calling `Cleanup()` on a child context doesn't affect the parent:

```go
parentDC := ctxdep.NewDependencyContext(ctx, ctxdep.WithCleanup(), parentResource)
childDC := ctxdep.NewDependencyContext(parentDC, ctxdep.WithCleanup(), childResource)

childDC.Cleanup()   // Only cleans childResource
parentDC.Cleanup()  // Cleans parentResource
```

### Generator Dependencies

Dependencies created by generators are also cleaned up:

```go
dc := ctxdep.NewDependencyContext(ctx,
    ctxdep.WithCleanup(),
    func() *DatabaseConnection {
        return &DatabaseConnection{conn: openDB()}
    },
)

// Access the generated dependency
_ = ctxdep.Get[*DatabaseConnection](dc)

dc.Cleanup()  // Closes the generated DatabaseConnection
```

### Custom Cleanup Precedence

Custom cleanup functions take precedence over `io.Closer`:

```go
type Resource struct{}

func (r *Resource) Close() error {
    fmt.Println("io.Closer called")
    return nil
}

dc := ctxdep.NewDependencyContext(ctx,
    ctxdep.WithCleanupFunc(func(r *Resource) {
        fmt.Println("Custom cleanup called")
    }),
    &Resource{},
)
dc.Cleanup()  // Prints "Custom cleanup called", NOT "io.Closer called"
```

## Why Explicit Cleanup?

The library uses explicit cleanup instead of automatic cleanup (e.g., on context cancellation) because:

1. **Race condition prevention**: Avoids cleanup happening during concurrent access
2. **Predictable behavior**: You control exactly when cleanup occurs
3. **Simpler mental model**: No surprises about resource lifetimes

---

## See Also

- [Getting Started](getting-started.md) - Basic context creation
- [Advanced](advanced.md) - Thread safety considerations
