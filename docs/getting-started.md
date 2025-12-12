# Getting Started

This guide will get you up and running with go-ctxdep in about 5 minutes.

## Installation

```bash
go get github.com/gburgyan/go-ctxdep
```

## Your First Dependency

The simplest case is to put an object into the dependency context, then pull it out later:

```go
type MyData struct {
    Value string
}

func Processor(ctx context.Context) {
    dc := ctxdep.NewDependencyContext(ctx, &MyData{Value: "for later"})
    client(dc)
}

func client(ctx context.Context) {
    data := ctxdep.Get[*MyData](ctx)
    fmt.Printf("Here's the data: %s", data.Value)
}
```

This works similarly to `context.WithValue()`: you add something to the context, pass it around, and pull things out of it.

**Key point:** The client code never changes in how it works. You always ask for an object from the context, and you receive it - it doesn't matter how that object got into the context.

## Working with Interfaces

The same process works with interfaces:

```go
type Service interface {
    Call(i int) int
}

type ServiceCaller struct{}

func (s *ServiceCaller) Call(i int) int {
    return i * 2
}

func Processor(ctx context.Context) {
    dc := ctxdep.NewDependencyContext(ctx, &ServiceCaller{})
    client(dc)
}

func client(ctx context.Context) {
    service := ctxdep.Get[Service](ctx)
    result := service.Call(42)
}
```

The dependency context is smart enough to realize that `*ServiceCaller` implements the `Service` interface. When asked to retrieve `Service`, it returns the instance cast to the interface type.

## Slices of Inputs

When organizing dependencies across components, you can pass slices:

```go
func componentADeps() []any {
    return []any{ /* objects and generators */ }
}

func componentBDeps() []any {
    return []any{ /* objects and generators */ }
}

func Processor(ctx context.Context) {
    dc := ctxdep.NewDependencyContext(ctx, componentADeps(), componentBDeps())
    client(dc)
}
```

If a `[]any` is passed, those are flattened and evaluated as if they weren't in a sub-slice. This prevents having to manually concatenate slices.

## The DependencyContext Type

`NewDependencyContext` returns a `*DependencyContext` that implements the `context.Context` interface. You can use it anywhere a context is expected while also having direct access to methods like `Cleanup()`:

```go
dc := ctxdep.NewDependencyContext(ctx, myService)

// Use as a regular context
handleRequest(dc)

// Access DependencyContext methods directly
defer dc.Cleanup()
```

## What's Next?

- **[Core Concepts](core-concepts.md)** - Understand how context dependencies work
- **[Generators](generators.md)** - The real power: lazy dependency creation
- **[Testing](testing.md)** - Why this library makes testing easy

---

## See Also

- [Core Concepts](core-concepts.md) - Deeper understanding of the fundamentals
- [Generators](generators.md) - Lazy dependency creation
