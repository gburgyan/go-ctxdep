# Advanced Topics

This guide covers thread safety, debugging, timing integration, and other advanced topics.

## Thread Safety

All operations on the dependency context are thread-safe.

### Concurrent Generator Execution

If two goroutines simultaneously request a dependency that requires a generator:
1. One goroutine runs the generator
2. The other goroutine blocks until the generator completes
3. Both receive the same cached result

```go
ctx := ctxdep.NewDependencyContext(ctx, func() *ExpensiveResource {
    fmt.Println("Generator called")  // Prints only once
    return &ExpensiveResource{}
})

var wg sync.WaitGroup
for i := 0; i < 10; i++ {
    wg.Add(1)
    go func() {
        defer wg.Done()
        resource := ctxdep.Get[*ExpensiveResource](ctx)
        // All goroutines get the same resource
    }()
}
wg.Wait()
```

### Why This Matters

- Generators often involve expensive operations (DB queries, API calls)
- Running them multiple times would be wasteful
- The locking ensures exactly-once execution

## Cyclic Dependency Detection

The library detects circular dependencies at runtime:

```go
// A needs B, B needs A - this is detected and causes an error
genA := func(b *B) *A { return &A{} }
genB := func(a *A) *B { return &B{} }

ctx := ctxdep.NewDependencyContext(ctx, genA, genB)
ctxdep.Get[*A](ctx)  // Panics with cycle detection error
```

**Why detection instead of prevention?**
- Cycles are detected when resolution is attempted
- This catches complex cycles involving multiple contexts
- Without detection, cycles would cause deadlocks

## Debugging with Status()

`Status()` returns a string describing everything in the context:

```go
c1 := ctxdep.NewDependencyContext(context.Background(),
    func() *TestImpl { return &TestImpl{val: 42} },
    func() *TestDoodad { return &TestDoodad{val: "woot"} },
)

c2 := ctxdep.NewDependencyContext(c1,
    func(in TestInterface) *TestWidget { return &TestWidget{val: in.getVal()} },
    &TestDoodad{val: "something cool"},
)

widget := ctxdep.Get[*TestWidget](c2)
fmt.Println(ctxdep.Status(c2))
```

Output:
```
*ctxdep.testDoodad - direct value set
*ctxdep.testWidget - created from generator: (ctxdep.testInterface) *ctxdep.testWidget
ctxdep.testInterface - imported from parent context
----
parent dependency context:
*ctxdep.testDoodad - uninitialized - generator: () *ctxdep.testDoodad
*ctxdep.testImpl - created from generator: () *ctxdep.testImpl
ctxdep.testInterface - assigned from *ctxdep.testImpl
```

### Reading Status Output

| Status | Meaning |
|--------|---------|
| `direct value set` | Concrete value added directly |
| `created from generator: (params) returns` | Generator was called |
| `uninitialized - generator: ...` | Generator registered but not yet called |
| `imported from parent context` | Value comes from parent |
| `assigned from *Type` | Interface satisfied by concrete type |

## Error Handling

### DependencyError Type

Errors from the dependency context are of type `DependencyError`:

```go
type DependencyError struct {
    Message        string
    ReferencedType reflect.Type
    Status         string
    SourceError    error
}
```

- `Message`: Description of what went wrong
- `ReferencedType`: The type that was being requested
- `Status`: Output of `Status()` at error time
- `SourceError`: Underlying error (if any)

### Handling Errors

```go
user, err := ctxdep.GetWithError[*User](ctx)
if err != nil {
    var depErr *ctxdep.DependencyError
    if errors.As(err, &depErr) {
        log.Printf("Failed to get %v: %s\nContext status:\n%s",
            depErr.ReferencedType,
            depErr.Message,
            depErr.Status,
        )
    }
}
```

## Secure Context Architecture

The library uses a dual-context architecture for security:

```go
type secureContext struct {
    baseContext   context.Context  // Contains dependencies
    timingContext context.Context  // Contains timing/deadlines
}
```

**Why separate contexts?**
- Prevents child contexts from accessing parent dependencies through timing context
- Prevents data pollution between contexts
- Generators run from their creation context, not the calling context

This means:
- Deadlines and cancellation from the caller's context are honored
- Dependency resolution uses the creation context
- Child dependencies can't "pollute" parent generators

## Timing Integration

The library integrates with [go-timing](https://github.com/gburgyan/go-timing) for performance monitoring.

### Enabling Timing

```go
// Disabled by default
ctxdep.TimingMode = ctxdep.TimingDisable

// Create timing context for immediate dependencies
ctxdep.TimingMode = ctxdep.TimingImmediate

// Also time individual generator calls
ctxdep.TimingMode = ctxdep.TimingGenerators
```

### TimingImmediate Output

```text
[ImmediateDeps] > CtxGen(*ctxdep.testWidget) - 101.05ms (generator:() *ctxdep.testWidget)
CtxGen(*ctxdep.testWidget) - 49.98ms (generator:() *ctxdep.testWidget, wait:parallel)
```

This shows:
- The immediate generator took ~100ms
- A later access waited ~50ms for the already-running generator
- `wait:parallel` indicates blocking on another goroutine

### TimingGenerators Output

```text
CtxGen(*ctxdep.testDoodad) - 30.08µs (generator:(context.Context, *ctxdep.testWidget) *ctxdep.testDoodad)
CtxGen(*ctxdep.testDoodad) > CtxGen(*ctxdep.testWidget) - 3.08µs (generator:(context.Context) *ctxdep.testWidget)
```

Shows nested generator calls and their timing.

## Edge Cases

### Multiple Types Implementing Same Interface

If multiple types in the context implement the same interface, which one is returned is **undefined**:

```go
ctx := ctxdep.NewDependencyContext(ctx,
    &ServiceA{},  // Implements Service
    &ServiceB{},  // Also implements Service
)

// Which one? Undefined behavior!
svc := ctxdep.Get[Service](ctx)
```

**Solution:** Be explicit about which implementation you want, or structure your context to avoid this.

### Performance Considerations

The library uses reflection heavily but includes caching:

- Type information is cached in `type_cache.go`
- Interface satisfaction checks are cached
- Repeated lookups for the same type are fast

For performance-critical code:
- Use `Get` directly rather than `GetWithError` when you don't need error handling
- Consider `Immediate()` for dependencies that are always needed
- Use caching for expensive generators

---

## See Also

- [Design Decisions](design-decisions.md) - Why the library works this way
- [Generators](generators.md) - Generator patterns
- [Testing](testing.md) - Debugging tests with Status()
