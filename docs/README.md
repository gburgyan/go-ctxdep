# go-ctxdep Documentation

Context-based dependency management for Go - simple, type-safe, and test-friendly.

## Quick Links

| New to go-ctxdep? | Already using it? |
|-------------------|-------------------|
| [Getting Started](getting-started.md) | [Generators Guide](generators.md) |
| [Core Concepts](core-concepts.md) | [Testing Guide](testing.md) |

## Documentation

### Getting Started

- **[Getting Started](getting-started.md)** - Installation and 5-minute quick start
- **[Core Concepts](core-concepts.md)** - How context dependencies work

### Key Features

- **[Generators](generators.md)** - Lazy dependency creation with automatic chaining
- **[Testing](testing.md)** - Why this library makes testing easy
- **[Adapters](adapters.md)** - Partial function application with context dependencies
- **[Caching](caching.md)** - Cache expensive generator results

### Additional Features

- **[Validation](validation.md)** - Validate dependencies during context creation
- **[Lifecycle Management](lifecycle.md)** - Resource cleanup with io.Closer support
- **[Context Control](context-control.md)** - Locking, overrides, and production safety
- **[Optional Dependencies](optional-deps.md)** - Graceful handling of missing dependencies

### Advanced Topics

- **[Advanced](advanced.md)** - Thread safety, debugging, timing integration
- **[Design Decisions](design-decisions.md)** - Architectural rationale and philosophy
