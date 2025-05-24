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
package ctxdep
