// Package ctxdep provides a convenient way to add dependencies to a context. It has the ability to have
// simple objects in the context to be asked for by type as well as objects that implement an interface to
// be retrieved by the interface type. In addition, it supports generator functions that can lazily create
// either.
//
// The DependencyContext object has comprehensive documentation about how it works.
//
// There are also helper global functions that make using this more concise.
//
// # Optional Dependencies
//
// The package now supports optional dependency retrieval that doesn't panic when dependencies are missing:
//
//	value, found := ctxdep.GetOptional[*MyService](ctx)
//	if !found {
//	    // Handle missing dependency gracefully
//	}
//
// # Lifecycle Management
//
// Dependencies can have cleanup functions that are automatically called when the context is cancelled.
// This feature must be explicitly enabled using WithCleanup() or WithCleanupFunc():
//
//	// Enable automatic cleanup for io.Closer types
//	ctx = ctxdep.NewDependencyContext(ctx,
//	    ctxdep.WithCleanup(),
//	    closableService,
//	)
//
//	// Or with a custom cleanup function
//	ctx = ctxdep.NewDependencyContext(ctx,
//	    ctxdep.WithCleanupFunc(func(s *MyService) {
//	        s.Shutdown()
//	    }),
//	    service,
//	)
//
// Types implementing io.Closer will have their Close() method called automatically when
// WithCleanup() is used. Custom cleanup functions take precedence over automatic cleanup.
//
// # Adapter Functions
//
// The package supports adapter functions that combine dependency injection with partial application.
// An adapter wraps a function to inject some parameters from the dependency context while leaving
// others to be provided at call time:
//
//	// Define an adapter type
//	type UserAdapter func(ctx context.Context, userID string) (*User, error)
//
//	// Create a function that needs dependencies
//	func lookupUser(ctx context.Context, db *Database, userID string) (*User, error) {
//	    return db.GetUser(ctx, userID)
//	}
//
//	// Register the adapter
//	ctx = ctxdep.NewDependencyContext(ctx,
//	    db,
//	    ctxdep.Adapt[UserAdapter](lookupUser),
//	)
//
//	// Use the adapter
//	adapter := ctxdep.Get[UserAdapter](ctx)
//	user, err := adapter(ctx, "user123")
//
// Adapters are validated during context initialization to ensure all dependencies can be resolved.
// They cannot be used as generators for other dependencies - they are specifically for creating
// partially applied functions. For security, adapters capture dependencies from their creation
// context, preventing child contexts from overriding critical dependencies.
//
// Anonymous Function Types:
//
// While named function types are recommended for clarity, adapters also work with anonymous
// function types:
//
//	ctx = ctxdep.NewDependencyContext(ctx,
//	    db,
//	    ctxdep.Adapt[func(context.Context, string) (*User, error)](lookupUser),
//	)
//	adapter := ctxdep.Get[func(context.Context, string) (*User, error)](ctx)
//
// Note that Go treats anonymous function types as identical based on their signature, ignoring
// parameter names. These two types are considered the same:
//   - func(ctx context.Context, id string) (*User, error)
//   - func(context.Context, string) (*User, error)
//
// Regular functions can also be stored as dependencies (as pointers), but they won't have
// the partial application behavior of adapters:
//
//	regularFunc := func(id string) *User { return &User{ID: id} }
//	ctx = ctxdep.NewDependencyContext(ctx, &regularFunc)
//	fn := ctxdep.Get[*func(string) *User](ctx)
package ctxdep
