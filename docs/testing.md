# Testing Guide

Testing is what started the idea for this library. This guide explains why go-ctxdep makes testing dramatically easier and shows comprehensive testing patterns.

## The Problem

Without a system for managing dependencies through context, overriding them in a unit test can be awkward. Consider this common pattern:

```go
package user

func GetUserData(userId string) *UserData { /* calls database */ }

package security

func isPermitted(request *Request) bool {
    userData := user.GetUserData(request.userId)
    if userData.IsAdmin {
        return true
    }
    // other stuff...
    return false
}
```

This works in production, but it's hard to test `isPermitted` without calling the real `GetUserData`.

## Traditional Approaches (And Their Problems)

### Global Variables

```go
package user

type UserFetcher interface {
    GetUserData(userId string) *UserData
}

var Fetcher UserFetcher = &DefaultUserFetcher{}
```

**Problem:** Running tests in parallel causes race conditions. Tests interfere with each other as they swap out the global.

### Interfaces Everywhere

Create interfaces for everything and pass them explicitly.

**Problem:** Changes production code structure just for testing. Function signatures grow as more dependencies are added.

### Mocking Frameworks

Use code generation or reflection-based mocking.

**Problem:** Verbose, brittle, runtime reflection, and tests become harder to understand.

## The Solution: Context Dependencies

With go-ctxdep, the same code becomes:

```go
func isPermitted(ctx context.Context) bool {
    user := ctxdep.Get[*UserData](ctx)
    if user.IsAdmin {
        return true
    }
    // other stuff...
    return false
}

func Test_isPermitted(t *testing.T) {
    user := &UserData{
        ID:      42,
        Name:    "George",
        IsAdmin: true,
    }
    ctx := ctxdep.NewDependencyContext(context.Background(), user)

    permitted := isPermitted(ctx)

    assert.True(t, permitted)
}
```

Each test gets its own context with exactly the dependencies it needs. No global state, no race conditions, no mocking frameworks.

## Basic Test Setup

The simplest pattern is providing mock data directly:

```go
func TestUserProcessing(t *testing.T) {
    // Create test dependencies
    mockUser := &User{ID: "123", Name: "Test User"}
    mockDB := &MockDatabase{users: map[string]*User{"123": mockUser}}

    // Create context with test dependencies
    ctx := ctxdep.NewDependencyContext(context.Background(),
        mockUser,
        mockDB,
    )

    // Run the code under test
    result := ProcessUser(ctx)

    // Assert results
    assert.Equal(t, "Test User processed", result)
}
```

## Table-Driven Tests

go-ctxdep works beautifully with table-driven tests:

```go
func TestPermissions(t *testing.T) {
    tests := []struct {
        name     string
        user     *UserData
        resource string
        want     bool
    }{
        {
            name:     "admin can access anything",
            user:     &UserData{IsAdmin: true},
            resource: "secret",
            want:     true,
        },
        {
            name:     "regular user cannot access secret",
            user:     &UserData{IsAdmin: false},
            resource: "secret",
            want:     false,
        },
        {
            name:     "regular user can access public",
            user:     &UserData{IsAdmin: false},
            resource: "public",
            want:     true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ctx := ctxdep.NewDependencyContext(context.Background(), tt.user)
            got := canAccess(ctx, tt.resource)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

## Testing with Adapters

Adapters are particularly powerful for testing. They let you replace database access and external calls with simple functions.

### The Problem

Consider a function that checks document permissions:

```go
type PermissionService struct {
    db *sql.DB
}

func (s *PermissionService) CanUserEditDocument(userID, documentID string) (bool, error) {
    doc, err := s.getDocument(documentID)
    if err != nil {
        return false, err
    }
    if doc.OwnerID == userID {
        return true, nil
    }
    return s.checkPermission(userID, "documents:edit")
}
```

Testing this traditionally requires a real database or complex mocking.

### The Solution

Define adapter types and use them in your business logic:

```go
// Define adapters for external dependencies
type PermissionChecker func(ctx context.Context, userID, resource string) (bool, error)
type DocumentGetter func(ctx context.Context, documentID string) (*Document, error)

// Implement real database functions
func checkPermissionDB(ctx context.Context, db *sql.DB, userID, resource string) (bool, error) {
    var has bool
    err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM permissions WHERE user_id = ? AND resource = ?)",
        userID, resource).Scan(&has)
    return has, err
}

func getDocumentDB(ctx context.Context, db *sql.DB, documentID string) (*Document, error) {
    // ... database query
}

// Business logic uses adapters from context
func CanUserEditDocument(ctx context.Context, userID, documentID string) (bool, error) {
    checkPermission := ctxdep.Get[PermissionChecker](ctx)
    getDocument := ctxdep.Get[DocumentGetter](ctx)

    doc, err := getDocument(ctx, documentID)
    if err != nil {
        return false, err
    }
    if doc.OwnerID == userID {
        return true, nil
    }
    return checkPermission(ctx, userID, "documents:edit")
}

// Production setup
func setupProduction(db *sql.DB) context.Context {
    return ctxdep.NewDependencyContext(context.Background(),
        db,
        ctxdep.Adapt[PermissionChecker](checkPermissionDB),
        ctxdep.Adapt[DocumentGetter](getDocumentDB),
    )
}
```

### Testing Becomes Trivial

```go
func TestCanUserEditDocument(t *testing.T) {
    tests := []struct {
        name          string
        userID        string
        documentID    string
        setupContext  func() context.Context
        expectedAllow bool
        expectedError bool
    }{
        {
            name:       "owner can edit",
            userID:     "user123",
            documentID: "doc456",
            setupContext: func() context.Context {
                getDoc := func(ctx context.Context, docID string) (*Document, error) {
                    return &Document{ID: docID, OwnerID: "user123"}, nil
                }
                checkPerm := func(ctx context.Context, userID, resource string) (bool, error) {
                    t.Fatal("should not check permissions for owner")
                    return false, nil
                }
                return ctxdep.NewDependencyContext(context.Background(),
                    ctxdep.Adapt[DocumentGetter](getDoc),
                    ctxdep.Adapt[PermissionChecker](checkPerm),
                )
            },
            expectedAllow: true,
        },
        {
            name:       "non-owner with permission",
            userID:     "user789",
            documentID: "doc456",
            setupContext: func() context.Context {
                getDoc := func(ctx context.Context, docID string) (*Document, error) {
                    return &Document{ID: docID, OwnerID: "user123"}, nil
                }
                checkPerm := func(ctx context.Context, userID, resource string) (bool, error) {
                    return userID == "user789" && resource == "documents:edit", nil
                }
                return ctxdep.NewDependencyContext(context.Background(),
                    ctxdep.Adapt[DocumentGetter](getDoc),
                    ctxdep.Adapt[PermissionChecker](checkPerm),
                )
            },
            expectedAllow: true,
        },
        {
            name:       "database error",
            userID:     "user789",
            documentID: "doc456",
            setupContext: func() context.Context {
                getDoc := func(ctx context.Context, docID string) (*Document, error) {
                    return nil, errors.New("database connection lost")
                }
                checkPerm := func(ctx context.Context, userID, resource string) (bool, error) {
                    return false, nil
                }
                return ctxdep.NewDependencyContext(context.Background(),
                    ctxdep.Adapt[DocumentGetter](getDoc),
                    ctxdep.Adapt[PermissionChecker](checkPerm),
                )
            },
            expectedAllow: false,
            expectedError: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ctx := tt.setupContext()
            allowed, err := CanUserEditDocument(ctx, tt.userID, tt.documentID)

            if tt.expectedError {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
            }
            assert.Equal(t, tt.expectedAllow, allowed)
        })
    }
}
```

## Test Helper Patterns

Create reusable test context builders:

```go
func setupTestContext(mockDB *MockDatabase, mockCache *CacheService) context.Context {
    return ctxdep.NewDependencyContext(context.Background(),
        realLogger,                    // Always use real logger
        ctxdep.Optional(mockDB),       // Use mock if provided
        ctxdep.Optional(mockCache),    // Use mock if provided
    )
}

func TestWithMockDB(t *testing.T) {
    mockDB := &MockDatabase{...}
    ctx := setupTestContext(mockDB, nil)  // Only mock the DB
    // Test code...
}

func TestWithBothMocks(t *testing.T) {
    ctx := setupTestContext(&MockDatabase{}, &CacheService{})
    // Test code...
}
```

## Parallel Testing

Because each test has its own context, parallel testing is safe:

```go
func TestParallel(t *testing.T) {
    tests := []struct {
        name string
        user *User
    }{
        {"admin", &User{IsAdmin: true}},
        {"regular", &User{IsAdmin: false}},
        {"guest", nil},
    }

    for _, tt := range tests {
        tt := tt  // Capture range variable
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()  // Safe! Each test has isolated context

            ctx := ctxdep.NewDependencyContext(context.Background(),
                ctxdep.Optional(tt.user),
            )
            // Test code...
        })
    }
}
```

## Skip Generators in Tests

A key insight: **in tests, you don't use generators at all.** Just provide the values directly.

Production code might use a generator that does expensive database lookups:

```go
// Production setup
ctx := ctxdep.NewDependencyContext(ctx,
    database,
    request,
    UserDataGenerator,  // Hits the database
)
```

But in tests, skip the generator and provide the result:

```go
func TestBusinessLogic(t *testing.T) {
    // No generator, no database - just the data you need
    ctx := ctxdep.NewDependencyContext(context.Background(),
        &UserData{ID: 123, Name: "Test User", IsAdmin: true},
    )

    result := processUser(ctx)
    assert.Equal(t, expected, result)
}
```

The code under test calls `Get[*UserData](ctx)` and has no idea whether it came from a generator or was placed directly. This means:

- **Test business logic, not infrastructure**: Your tests verify behavior, not database queries
- **Fast tests**: No I/O, no network calls
- **Simple setup**: Just create the structs you need

## Testing Validation

Test validation logic:

```go
func TestValidation(t *testing.T) {
    t.Run("valid user passes", func(t *testing.T) {
        user := &User{Age: 25}
        ctx, err := ctxdep.NewDependencyContextWithValidation(context.Background(),
            user,
            ctxdep.Validate(func(u *User) error {
                if u.Age < 18 {
                    return errors.New("must be 18+")
                }
                return nil
            }),
        )
        assert.NoError(t, err)
        assert.NotNil(t, ctx)
    })

    t.Run("invalid user fails", func(t *testing.T) {
        user := &User{Age: 16}
        _, err := ctxdep.NewDependencyContextWithValidation(context.Background(),
            user,
            ctxdep.Validate(func(u *User) error {
                if u.Age < 18 {
                    return errors.New("must be 18+")
                }
                return nil
            }),
        )
        assert.Error(t, err)
        assert.Contains(t, err.Error(), "must be 18+")
    })
}
```

## Debugging Tests

When tests fail, use `Status()` to understand context state:

```go
func TestDebug(t *testing.T) {
    ctx := ctxdep.NewDependencyContext(context.Background(),
        &User{},
        func() *Config { return &Config{} },
    )

    // Print context state for debugging
    t.Log(ctxdep.Status(ctx))

    // Continue with test...
}
```

Output shows what's in the context and how it got there:

```
*User - direct value set
*Config - uninitialized - generator: () *Config
```

## Key Advantages

1. **No Mocking Frameworks** - Just simple functions that return what you need
2. **Parallel Testing** - Each test has its own context, no shared state
3. **Clear Test Intent** - The test setup shows exactly what each dependency does
4. **Type Safety** - Compiler ensures test adapters match expected signatures
5. **No Abstraction Breaking** - Business logic doesn't know it's being tested
6. **Lazy Evaluation** - Database connections only made if actually needed

Compare this to traditional approaches:
- **Interfaces everywhere**: Requires changing production code structure
- **Mocking frameworks**: Verbose setup, runtime reflection, less clear intent
- **Global variables**: Can't run tests in parallel, hidden dependencies

The adapter pattern preserves your code's natural structure while making it completely testable.

---

## See Also

- [Adapters](adapters.md) - Adapter pattern details
- [Generators](generators.md) - Mock generators in tests
- [Validation](validation.md) - Testing validation logic
- [Advanced](advanced.md) - Debugging with Status()
