package ctxdep

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

type LockTestLogger struct {
	name string
}

type LockTestDatabase struct {
	conn string
}

func TestWithLock(t *testing.T) {
	t.Run("locked context prevents WithOverrides", func(t *testing.T) {
		// Create a locked parent context
		parent := NewDependencyContext(context.Background(), WithLock())

		// Attempt to create child with WithOverrides should panic
		assert.PanicsWithValue(t, "cannot use WithOverrides on a context with a locked parent", func() {
			_ = NewDependencyContext(parent, WithOverrides())
		})
	})

	t.Run("locked context allows normal child contexts", func(t *testing.T) {
		// Create a locked parent context
		parent := NewDependencyContext(context.Background(), WithLock())

		// Creating child without WithOverrides should work
		child := NewDependencyContext(parent)
		assert.NotNil(t, child, "expected child context to be created successfully")
	})

	t.Run("locked grandparent prevents WithOverrides", func(t *testing.T) {
		// Create a locked grandparent context
		grandparent := NewDependencyContext(context.Background(), WithLock())
		parent := NewDependencyContext(grandparent)

		// Attempt to create grandchild with WithOverrides should panic
		assert.Panics(t, func() {
			_ = NewDependencyContext(parent, WithOverrides())
		})
	})

	t.Run("unlocked context allows WithOverrides", func(t *testing.T) {
		// Create an unlocked parent context
		parent := NewDependencyContext(context.Background())

		// Creating child with WithOverrides should work
		child := NewDependencyContext(parent, WithOverrides())
		assert.NotNil(t, child, "expected child context to be created successfully")
	})

	t.Run("Lock method locks context", func(t *testing.T) {
		// Create an unlocked context
		parent := NewDependencyContext(context.Background())

		// Lock it using the Lock() method
		parent.Lock()

		// Attempt to create child with WithOverrides should panic
		assert.PanicsWithValue(t, "cannot use WithOverrides on a context with a locked parent", func() {
			_ = NewDependencyContext(parent, WithOverrides())
		})
	})

	t.Run("global Lock function locks context", func(t *testing.T) {
		// Create an unlocked context
		parent := NewDependencyContext(context.Background())

		// Lock it using the global Lock() function
		Lock(parent)

		// Attempt to create child with WithOverrides should panic
		assert.PanicsWithValue(t, "cannot use WithOverrides on a context with a locked parent", func() {
			_ = NewDependencyContext(parent, WithOverrides())
		})
	})

	t.Run("Lock after creation for production safety", func(t *testing.T) {
		// Simulate a function that creates a context
		createAppContext := func() *DependencyContext {
			return NewDependencyContext(context.Background(),
				&LockTestLogger{name: "app"},
				&LockTestDatabase{conn: "prod"},
			)
		}

		// Test scenario: unlocked for tests
		testCtx := createAppContext()
		// Should be able to override in tests
		testChild := NewDependencyContext(testCtx, WithOverrides(), &LockTestLogger{name: "test"})
		logger := Get[*LockTestLogger](testChild)
		assert.Equal(t, "test", logger.name)

		// Production scenario: lock after creation
		prodCtx := createAppContext()
		prodCtx.Lock()

		// Should not be able to override in production
		assert.Panics(t, func() {
			_ = NewDependencyContext(prodCtx, WithOverrides(), &LockTestLogger{name: "hacker"})
		})
	})
}

func TestOverrideable(t *testing.T) {
	t.Run("overrideable dependency can be overridden in normal context", func(t *testing.T) {
		logger1 := &LockTestLogger{name: "logger1"}
		logger2 := &LockTestLogger{name: "logger2"}

		parent := NewDependencyContext(context.Background(),
			Overrideable(logger1),
		)

		// Should be able to override in child
		child := NewDependencyContext(parent, logger2)

		var result *LockTestLogger
		Get[*LockTestLogger](child)
		result = Get[*LockTestLogger](child)

		assert.Equal(t, "logger2", result.name)
	})

	t.Run("overrideable dependency can be overridden in locked context", func(t *testing.T) {
		logger1 := &LockTestLogger{name: "logger1"}
		logger2 := &LockTestLogger{name: "logger2"}
		db := &LockTestDatabase{conn: "main"}

		parent := NewDependencyContext(context.Background(),
			WithLock(),
			Overrideable(logger1),
			db,
		)

		// Should be able to override logger but not database
		child := NewDependencyContext(parent, logger2)

		var resultLogger *LockTestLogger
		resultLogger = Get[*LockTestLogger](child)

		assert.Equal(t, "logger2", resultLogger.name)

		// Database should still be from parent
		var resultDB *LockTestDatabase
		resultDB = Get[*LockTestDatabase](child)

		assert.Equal(t, "main", resultDB.conn)
	})

	t.Run("non-overrideable dependency cannot be overridden in locked context", func(t *testing.T) {
		db1 := &LockTestDatabase{conn: "db1"}
		db2 := &LockTestDatabase{conn: "db2"}

		parent := NewDependencyContext(context.Background(),
			WithLock(),
			db1, // Not marked as overrideable
		)

		// Attempt to override should panic
		assert.Panics(t, func() {
			_ = NewDependencyContext(parent, db2)
		})
	})

	t.Run("overrideable works with generators", func(t *testing.T) {
		genLogger1 := func() *LockTestLogger {
			return &LockTestLogger{name: "gen1"}
		}

		genLogger2 := func() *LockTestLogger {
			return &LockTestLogger{name: "gen2"}
		}

		parent := NewDependencyContext(context.Background(),
			WithLock(),
			Overrideable(genLogger1),
		)

		// Should be able to override with another generator
		child := NewDependencyContext(parent, genLogger2)

		var result *LockTestLogger
		result = Get[*LockTestLogger](child)

		assert.Equal(t, "gen2", result.name)
	})

	t.Run("overrideable inheritance from parent", func(t *testing.T) {
		logger1 := &LockTestLogger{name: "logger1"}
		logger2 := &LockTestLogger{name: "logger2"}
		logger3 := &LockTestLogger{name: "logger3"}

		// Mark as overrideable in grandparent
		grandparent := NewDependencyContext(context.Background(),
			Overrideable(logger1),
		)

		// Override in parent
		parent := NewDependencyContext(grandparent, logger2)

		// Should still be overrideable in child
		child := NewDependencyContext(parent, logger3)

		var result *LockTestLogger
		result = Get[*LockTestLogger](child)

		assert.Equal(t, "logger3", result.name)
	})

	t.Run("multiple overrideable dependencies", func(t *testing.T) {
		logger := &LockTestLogger{name: "logger"}
		db := &LockTestDatabase{conn: "db"}

		parent := NewDependencyContext(context.Background(),
			WithLock(),
			Overrideable(logger, db), // Both are overrideable
		)

		newLogger := &LockTestLogger{name: "newLogger"}
		newDB := &LockTestDatabase{conn: "newDB"}

		// Should be able to override both
		child := NewDependencyContext(parent, newLogger, newDB)

		var resultLogger *LockTestLogger
		resultLogger = Get[*LockTestLogger](child)
		assert.Equal(t, "newLogger", resultLogger.name)

		var resultDB *LockTestDatabase
		resultDB = Get[*LockTestDatabase](child)
		assert.Equal(t, "newDB", resultDB.conn)
	})

	t.Run("cannot use overrideable to override non-overrideable dependency", func(t *testing.T) {
		db := &LockTestDatabase{conn: "original"}

		// Create parent with non-overrideable database
		parent := NewDependencyContext(context.Background(), db)

		// Attempt to mark it as overrideable in child should not work
		// This should panic because db is not overrideable in parent
		assert.PanicsWithValue(t, "cannot mark type *ctxdep.LockTestDatabase as overrideable: already exists in parent context", func() {
			_ = NewDependencyContext(parent, Overrideable(&LockTestDatabase{conn: "new"}))
		})
	})

	t.Run("overrideable in child does not affect parent non-overrideable", func(t *testing.T) {
		logger := &LockTestLogger{name: "original"}

		// Create grandparent with non-overrideable logger
		grandparent := NewDependencyContext(context.Background(), logger)

		// This should fail because grandparent's logger is not overrideable
		// Create parent that tries to make it overrideable - should panic
		assert.PanicsWithValue(t, "cannot mark type *ctxdep.LockTestLogger as overrideable: already exists in parent context", func() {
			_ = NewDependencyContext(grandparent, Overrideable(&LockTestLogger{name: "parent"}))
		})
	})

	t.Run("overrideable must be declared before first use", func(t *testing.T) {
		logger1 := &LockTestLogger{name: "logger1"}
		logger2 := &LockTestLogger{name: "logger2"}

		// First context has logger as non-overrideable
		ctx1 := NewDependencyContext(context.Background(), logger1)

		// Second context tries to override - should fail
		assert.Panics(t, func() {
			_ = NewDependencyContext(ctx1, logger2)
		})
	})
}
