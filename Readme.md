![Build status](https://github.com/gburgyan/go-ctxdep/actions/workflows/go.yml/badge.svg) [![Go Report Card](https://goreportcard.com/badge/github.com/gburgyan/go-ctxdep)](https://goreportcard.com/report/github.com/gburgyan/go-ctxdep) [![PkgGoDev](https://pkg.go.dev/badge/github.com/gburgyan/go-ctxdep)](https://pkg.go.dev/github.com/gburgyan/go-ctxdep)

# go-ctxdep

Context-based dependency management for Go - simple, type-safe, and test-friendly.

Go already has a nice way to keep track of things with `context.Context`. This library adds helpers to simplify getting things out of that context, with support for lazy initialization, caching, and clean testing patterns.

## Installation

```bash
go get github.com/gburgyan/go-ctxdep
```

## Quick Example

```go
type UserService struct {
    // ...
}

func main() {
    ctx := ctxdep.NewDependencyContext(context.Background(), &UserService{})
    handleRequest(ctx)
}

func handleRequest(ctx context.Context) {
    svc := ctxdep.Get[*UserService](ctx)
    // use svc...
}
```

## Design Goals

* Simple interface built on Go's `context.Context`
* Type-safe access to dependencies
* Thread-safe with no deadlock risks
* Fail-fast - errors surface immediately, not in production
* Fast dependency resolution
* Explicit over magic - no hidden configuration
* **Easy testing** - the main motivation for this library

## Documentation

Full documentation is available in the [docs/](docs/) directory:

### Getting Started
- **[Getting Started](docs/getting-started.md)** - Installation and quick start
- **[Core Concepts](docs/core-concepts.md)** - How context dependencies work

### Key Features
- **[Generators](docs/generators.md)** - Lazy dependency creation
- **[Testing](docs/testing.md)** - Why this makes testing easy
- **[Adapters](docs/adapters.md)** - Partial function application
- **[Caching](docs/caching.md)** - Cache expensive operations

### Additional Features
- **[Validation](docs/validation.md)** - Validate during context creation
- **[Lifecycle](docs/lifecycle.md)** - Resource cleanup
- **[Context Control](docs/context-control.md)** - Locking and overrides
- **[Optional Dependencies](docs/optional-deps.md)** - Graceful handling

### Advanced
- **[Advanced Topics](docs/advanced.md)** - Thread safety, debugging, timing
- **[Design Decisions](docs/design-decisions.md)** - Architecture rationale

## Why This Library?

Testing. Testing is what started the idea for this.

Without a way to manage dependencies through context, overriding them in tests is awkward:

```go
// Hard to test - how do you mock GetUserData?
func isPermitted(request *Request) bool {
    userData := user.GetUserData(request.userId)
    return userData.IsAdmin
}
```

With go-ctxdep:

```go
func isPermitted(ctx context.Context) bool {
    user := ctxdep.Get[*UserData](ctx)
    return user.IsAdmin
}

func Test_isPermitted(t *testing.T) {
    ctx := ctxdep.NewDependencyContext(context.Background(),
        &UserData{IsAdmin: true},
    )
    assert.True(t, isPermitted(ctx))
}
```

Each test gets its own context. No global state. No race conditions. No mocking frameworks.

See the [Testing Guide](docs/testing.md) for comprehensive patterns.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
