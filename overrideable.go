package ctxdep

// overrideableWrapper is an internal wrapper to signal that these dependencies
// should be marked as overrideable in the DependencyContext.
type overrideableWrapper struct {
	dependencies []any
}

// Overrideable marks dependencies as always overrideable, even in locked contexts.
// This is useful for dependencies like loggers that may need to be replaced in
// testing scenarios even when the context is otherwise locked.
//
// A common use case is with logging libraries like slog, where you often want to
// add context to a logger by calling methods like With() which return a new logger:
//
//	logger := slog.Default()
//	ctx := NewDependencyContext(parent,
//	    WithLock(),                    // Lock the context
//	    Overrideable(logger),          // But logger can still be overridden
//	    database,                      // Normal dependency
//	)
//
//	// Later, add request context to logger
//	requestLogger := logger.With("request_id", requestID)
//	childCtx := NewDependencyContext(ctx, requestLogger)
func Overrideable(deps ...any) *overrideableWrapper {
	return &overrideableWrapper{
		dependencies: deps,
	}
}
