![Build status](https://github.com/gburgyan/go-ctxdep/actions/workflows/go.yml/badge.svg) [![Go Report Card](https://goreportcard.com/badge/github.com/gburgyan/go-ctxdep)](https://goreportcard.com/report/github.com/gburgyan/go-ctxdep) [![PkgGoDev](https://pkg.go.dev/badge/github.com/gburgyan/go-ctxdep)](https://pkg.go.dev/github.com/gburgyan/go-ctxdep)

# About

Go already has a nice way to keep track of things in your context: `context.Context`. This adds some helpers to that to simplify getting things out of that context that is already being passed around.

# Design Goals

* Offer a simple and easy to understand interface to the context dependencies
* Don't pass around new objects; the Golang context is just fine
  * Don't have things like added scopes or anything else like that
* Prioritize safety
  * Type safe access to objects in the context
  * Completely thread-safety with no chances of deadlocks even in extreme cases
  * No possibility of infinite loops when resolving more complex dependencies
  * Fail early and obviously--don't wait until some odd use case gets triggered in production
* Be fast at getting dependencies from the context
* Be explicit with what is added to the context--avoid magic and configuration
* Provide a flexible interface for adding things to the context
  * Make it easy to fetch things in the background to make more performant code
* Make it as easy to test code as possible
* Reduce boilerplate and unnecessary code
* Provide comprehensive debugging in case something _does_ get confusing

# Installation

```bash
go get github.com/gburgyan/go-ctxdep
```

# Features

The basic feature of the go-ctxdep module is to provide a simple way to access needed dependencies away from where those dependencies were created. It provides ways of putting dependencies, which are simply instances of objects that can be pulled out later, into the context that is already there, then accessing them simply afterward. For objects that may be more costly to produce, dependencies can be represented by generators that are called when they are first referenced. In cases where it is known that an expensive dependency is needed, you can mark the generator to run immediately which will fire off the generator in a background goroutine to be able to give it as much of a head start as possible. Additionally, you can also have a layer of caching on top of the generator so that the generator is only called once until the cache expires.

It builds on top of the existing `context.Context` features of Go offering some user-friendly features. What it doesn't do is try to offer a full dependency injection framework. It simply allows a flexible set of objects to be stored in the context.

By relying on the existing context framework of Go, it allows conveniently side-steps the need to have many of the features that dependency injection frameworks need. Instead, it uses the same features that Go programmers are already familiar with.

**Note:** `NewDependencyContext` now returns a `*DependencyContext` that implements the `context.Context` interface, allowing you to use it anywhere a context is expected while also providing direct access to methods like `Cleanup()`.

# Usage

## Basic usage

The simplest case is to put an object into the dependency context, then later on pull it out:

```Go
type MyData struct {
    value string
}

func Processor(ctx context.Context) {
    dc := ctxdep.NewDependencyContext(ctx, &MyData{data:"for later"})
    client(dc)
}

func client(ctx context.Context) {
    data := ctxdep.Get[*MyData](ctx)
    fmt.Printf("Here's the data: %s", data.value)
}
```

This works very similarly to how the base `context.WithValue()` system works: you add something to the context, pass it around to functions you call, and you pull things out of it.

A key point is that the client code above *never* changes in how it works. Fundamental to the design is that you always ask for an object out of the context, and you receive it--it doesn't matter how that object got into the context, it just works. There are a couple of ways of doing this operation, but it is always the same in concept.

## Slices of inputs

A slice of values can be passed in to the dependencies. If a `[]any` is passed, those are flattened and evaluated as if they weren't in a sub-slice. This is to support a use case where several components return `[]any` for their dependencies. This is simply a helper to prevent having to manually concatenate slices before passing them to `NewDependencyContext`.

```Go

func componentADeps() []any {
	return []any{ /* objects and generators */}
}

func componentBDeps() []any {
    return []any{ /* objects and generators */}
}

func Processor(ctx context.Context) {
    dc := ctxdep.NewDependencyContext(ctx, componentADeps(), componentBDeps())
    client(dc)
}

```

## Interfaces

The same process works with interfaces as well:

```Go
type Service interface {
   Call(i int) int
}

type ServiceCaller struct {
}

func (s *ServiceCaller) Call(i int) int {
    // Do stuff here...
}

func Processor(ctx context.Context) {
    dc := ctxdep.NewDependencyContext(ctx, &ServiceCaller{})
    client(dc)
}

func client(ctx context.Context) {
    service := ctxdep.Get[Service](ctx)
    service.Call(42)
}
```

The dependency context is smart enough to realize that the `ServiceCaller` type implements the `Service` interface. When asked to retrieve the `Service` object, it returns the instance that was added with `NewDependencyContext` cast to the `Service` type.

## Generators

When writing many services, it's common to have objects that represent things dealing with the specific request being processed. It may be the user's information, a product that is being viewed, or anything other similar types of object.

One of the typical ways of dealing with this is to either pass the request object around and look up the info as needed, or to look it up preemptively and put it in the `context` of the request.

This module solves this common use case by introducing the concept of a generator function. A generator function is simply a function that returns an object. We can add the generator function to the dependency context and the return types from that function are added to the dependency context. The generator is called to fill in the value if one of those types is requested. Once the value is known, it is stored in the dependency context eliminating future calls to the generator.

For example:

```Go
type UserDataService interface {
    Lookup(request Request) *UserData
}

type UserData struct {
    Id      int
    Name    string
    IsAdmin bool
}

type UserDataServiceCaller {
    // Implements UserDataService
}

func UserDataGenerator(request *Request) func(context.Context) (*UserData, error) {
    return func(ctx context.Context, userService *UserDataService) (*UserData, error) {
        userService := ctxdep.Get[*UserDataService](ctx)
        return userService.Lookup(request)
    }
}

func HandleRequest(ctx context.Context, request *Request) Response {
    ctx = ctxdep.NewDependencyContext(ctx, &UserDataServiceCaller{}, UserDataGenerator(request))
    isPermitted(ctx)
    ...
}

func isPermitted(ctx context.Context) bool {
    user := ctxdep.Get[*UserData](ctx)
    if user.IsAdmin {
        return true	
    }
    // other stuff...
    return false
}
```

When the `isPermitted` function asks for the `UserData` from the context, the function returned by `UserDataGenerator` is what's used to fulfill the request.

This also introduces a new concept: chained dependencies. The function returned by `UserDataGenerator` also requires an implementation of `UserDataService`. The dependency context sees this and resolves that dependency when calling the function.

The flexibility of this system is further examined in the section on testing.

However, the above example could be simpler. The function that returns a function seems awkward. What if we changed things to:

```Go
func UserDataGenerator(ctx context.Context) (*UserData, error) {
    userService := ctxdep.Get[*UserDataService](ctx)
    request := ctxdep.Get[*Request](ctx)
    return userService.Lookup(request)

}

func HandleRequest(ctx context.Context, request *Request) *Response {
    ctx = ctxdep.NewDependencyContext(ctx, &UserDataServiceCaller{}, request, UserDataGenerator)
    isPermitted(ctx)
    ...
}
```

While this works, it can be even simpler:

```Go
func UserDataGenerator(ctx context.Context, userService *UserDataService, request *Request) (*UserData, error) {
    return userService.Lookup(request)

}

// unchanged from before
func HandleRequest(ctx context.Context, request *Request) *Response {
    ctx = ctxdep.NewDependencyContext(ctx, &UserDataServiceCaller{}, request, UserDataGenerator)
    isPermitted(ctx)
    ...
}
```

The context dependencies figure out the parameters of the generators and uses the objects it has to provide the values for them.

## Immediate generators

A slight modification to the simple generators is the immediate generators. These work identically in all ways to the generators presented above, except the values for them are fetched immediately. This solves the use case of objects which are always required but are relatively expensive to get.

The only change to from above is:

```
func HandleRequest(ctx context.Context, request *Request) *Response {
    ctx = ctxdep.NewDependencyContext(ctx, &UserDataServiceCaller{}, request, ctxdep.Immediate(UserDataGenerator))
    isPermitted(ctx)
    ...
}
```

The immediate generator starts running in a new goroutine to fill in its results. While it is running, access to its results is blocked. This allows the long-running function that, for example is calling another service, to get a head start in execution. Without the `Immediate` specification, the first access to the `*UserData` would run the generator. With it, the generator starts running much quicker and the request for the `*UserData` will block for less time.

## Caching

The dependency context can be configured to cache the results of the generators. This is useful for objects that are expensive to generate but are not expected to change within the time-to-live of the cache.

To have the dependency context cache the results of a generator, simply add the `ctxdep.Cache` function to the generator:

```go

var cache = NewYourCacheType()

func UserDataGenerator(ctx context.Context, userService *UserDataService, request *Request) (*UserData, error) {
    return userService.Lookup(request)
}

func HandleRequest(ctx context.Context, request *Request) *Response {
    ctx = ctxdep.NewDependencyContext(ctx, &UserDataServiceCaller{}, 
	        request, ctxdep.Cache(cache, UserDataGenerator, time.Minute * 15))
    isPermitted(ctx)
    ...
}

```

In this case the call to the `UserDataGenerator` is wrapped in the `cache` call. This will cause the dependency context to cache the results of the generator for 15 minutes in this case. The results of this call will be cached in the `cache` object.

The inputs for the generator must implement the `ctxdep.Keyable` interface. This is:

```go
type Keyable interface {
    CacheKey() string
}

func (u *Request) CacheKey() string {
    return fmt.Sprintf("%d", u.Id)
}

// Similar for UserDataService. Perhaps the endpoint that's being called is a good key.
```

This will use the parameters that are passed to the generator to generate a key for the cache. The cache will then be used to store the results of the generator.

The `cache` object is expected to implement the `ctxdep.Cache` interface. The `ctxdep.Cache` interface is:

```go
type Cache interface {
    Get(ctx context.Context, key string) []any
    SetTTL(ctx context.Context, key string, value []any, ttl time.Duration)
}
```

There are many implementations of in-memory caches for Go, and it should be easy to adapt any of these to the `Cache` interface. If the cache needs to evict cache entries before the TTL expires, that is fine and expected. The only rule is that the `[]any` objects that are set using the `SetTTL` call, are equivalent to the `[]any` that are returned by the `Get`. 

The expectation is that this interface can wrap whatever caching system you want to use. Internally, there is a lock that will ensure that only a single call to the generator function will occur for each instance of a cache. This does not handle distributed locking if the cache provider is serializing to a shared resource. There is a specialized implementation similar to this cache for Redis that can be found in the related [go-rediscache](https://github.com/gburgyan/go-rediscache) package that offers more robust distributed locking, but specific to Redis.

## Cache key generation

The simplest way is to implement the `Keyable` interface as described above. If, for whatever reason, you can't implement that interface, there are several fallback options that are also attempted:

* You can call `ctxdep.RegisterCacheKeyProvider` with a custom function that will be called that generates the cache key.
* If the type implements the `Stringer` interface, that will be used to generate the cache key.
* The object is serialized using the default JSON serializer, and the result of that is used as the key.

## Pre-refreshing the cache

By initializing the cache by calling `CachedOpts`, you can enable some more advanced options. In addition to the TTL and duration provider mentioned earlier, this also exposes the `RefreshPercentage` option. This allows you to trigger a refresh of the cache in the background while returning the still valid cached results. If you set `RefreshPercentage` to 0.75, and access the cache 75% of the lifetime of a cache entry, the backing function will get called to refresh the cache. The refreshing occurs on a separate goroutine so the primary execution path is not delayed.

Even if multiple clients of the cache trigger a potential refresh, only a single refresh will occur.

## Lifecycle management

Dependencies can have cleanup functions that are automatically called when the context is cancelled. This is particularly useful for resources that need to be cleaned up like database connections, file handles, or network connections. This feature is opt-in and must be explicitly enabled using `WithCleanup()` or `WithCleanupFunc()`.

### Explicit cleanup for io.Closer

Any dependency that implements the `io.Closer` interface will have its `Close()` method called when you explicitly call `Cleanup()` on the dependency context, but only if cleanup is enabled:

```Go
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
        ctxdep.WithCleanup(), // Enable cleanup functionality
        dbConn,
    )
    defer dc.Cleanup() // Cleanup when function returns
    
    // Use the database connection
    // When the function returns, dbConn.Close() will be called
}
```

### Custom cleanup functions

For more complex cleanup logic or types that don't implement `io.Closer`, you can register custom cleanup functions:

```Go
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
    defer dc.Cleanup() // Explicitly cleanup when done
    
    // When the function returns, cleanupService will be invoked
}
```

## Adapter Functions

The library supports adapter functions that allow you to create partially applied functions where some parameters are filled from the dependency context while others are provided at call time.

### Basic adapter usage

```Go
type Database interface {
    GetUser(ctx context.Context, id string) (*User, error)
}

type User struct {
    ID    string
    Name  string
    Email string
}

// Define an adapter type
type UserAdapter func(ctx context.Context, userID string) (*User, error)

// Create a function that needs dependencies
func lookupUser(ctx context.Context, db Database, userID string) (*User, error) {
    return db.GetUser(ctx, userID)
}

func main() {
    ctx := context.Background()
    db := connectToDatabase() // Returns Database implementation
    
    // Register the adapter
    ctx = ctxdep.NewDependencyContext(ctx,
        db,
        ctxdep.Adapt[UserAdapter](lookupUser),
    )
    
    // Get and use the adapter
    adapter := ctxdep.Get[UserAdapter](ctx)
    user, err := adapter(ctx, "user123")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Printf("Found user: %+v\n", user)
}
```

### Adapters with multiple dependencies

Adapters can use multiple dependencies from the context:

```Go
type Config struct {
    APIKey string
}

type ComplexAdapter func(ctx context.Context, operation string, value int) (string, error)

func complexOperation(ctx context.Context, db Database, cfg *Config, operation string, value int) (string, error) {
    // Use both db and cfg along with the provided parameters
    user, _ := db.GetUser(ctx, cfg.APIKey)
    return fmt.Sprintf("%s: %s processed %d", operation, user.Name, value), nil
}

func main() {
    ctx := context.Background()
    db := connectToDatabase()
    cfg := &Config{APIKey: "admin"}
    
    ctx = ctxdep.NewDependencyContext(ctx,
        db,
        cfg,
        ctxdep.Adapt[ComplexAdapter](complexOperation),
    )
    
    adapter := ctxdep.Get[ComplexAdapter](ctx)
    result, _ := adapter(ctx, "process", 42)
    fmt.Println(result) // "process: Admin processed 42"
}
```

### Adapter validation

Adapters are validated during context initialization to ensure:
- All dependencies required by the original function can be resolved from the context
- The parameter and return types match between the adapter type and the original function
- The adapter type includes a `context.Context` parameter if the original function has one

```Go
// This will panic during context creation because Config is not available
ctx = ctxdep.NewDependencyContext(ctx,
    db, // Missing cfg!
    ctxdep.Adapt[ComplexAdapter](complexOperation),
)
```

### Important notes about adapters

1. Adapters cannot be used as generators for other dependencies - they are specifically for creating partially applied functions
2. For security reasons, adapters capture dependencies from the context where they were created, not from the context passed when calling the adapter. This prevents child contexts from overriding dependencies that the adapter depends on
3. All validation happens during `NewDependencyContext`, ensuring runtime safety

### Using anonymous function types

While named function types (like `UserAdapter`) are recommended for clarity and maintainability, you can also use anonymous function types with adapters:

```Go
// Using anonymous function type
ctx = ctxdep.NewDependencyContext(ctx,
    db,
    ctxdep.Adapt[func(context.Context, string) (*User, error)](lookupUser),
)

// Retrieve with the same anonymous type
adapter := ctxdep.Get[func(context.Context, string) (*User, error)](ctx)
```

**Important notes about anonymous function types:**

- Go considers anonymous function types identical based on their signature, not parameter names
- `func(ctx context.Context, id string) (*User, error)` and `func(context.Context, string) (*User, error)` are the same type
- Type aliases work as expected: `type MyFunc = func(context.Context, string) (*User, error)` can be used interchangeably with the expanded form
- The `Status()` output shows anonymous function types as their full signature

### Difference from regular function dependencies

Regular functions can be stored as dependencies (as pointers), but they don't provide partial application:

```Go
// Regular function stored as dependency
regularFunc := func(id string) *User { 
    return &User{ID: id} 
}
ctx = ctxdep.NewDependencyContext(ctx, &regularFunc)

// Retrieved as pointer
fn := ctxdep.Get[*func(string) *User](ctx)
user := (*fn)("123")  // Note: no dependency injection happens

// Adapter function - provides partial application
ctx = ctxdep.NewDependencyContext(ctx,
    db,
    ctxdep.Adapt[func(context.Context, string) (*User, error)](lookupUser),
)
adapter := ctxdep.Get[func(context.Context, string) (*User, error)](ctx)
user, err := adapter(ctx, "123")  // db is injected from context
```

## Validation

The library supports running validation functions during context creation. This is useful for ensuring that all dependencies meet certain criteria before proceeding with application logic.

### Basic validation usage

Validators are functions that return an error. They can take any dependencies from the context as parameters:

```go
// Define a validator function
func validateUser(ctx context.Context, user *User) error {
    if user.Age < 18 {
        return errors.New("user must be 18 or older")
    }
    return nil
}

// Create context with validation
ctx, err := ctxdep.NewDependencyContextWithValidation(parentCtx,
    &User{Name: "John", Age: 25},
    ctxdep.Validate(validateUser),
)
if err != nil {
    // Validation failed
    log.Fatal(err)
}

// If we get here, validation passed
user := ctxdep.Get[*User](ctx)
```

### Multiple validators

You can register multiple validators, and they will all be run:

```go
func validateAge(ctx context.Context, user *User) error {
    if user.Age < 18 {
        return errors.New("user must be 18 or older")
    }
    return nil
}

func validateEmail(ctx context.Context, user *User) error {
    if !strings.Contains(user.Email, "@") {
        return errors.New("invalid email format")
    }
    return nil
}

ctx, err := ctxdep.NewDependencyContextWithValidation(parentCtx,
    user,
    ctxdep.Validate(validateAge),
    ctxdep.Validate(validateEmail),
)
```

### Validators with dependencies

Validators can use multiple dependencies from the context:

```go
func validateOrderLimit(ctx context.Context, user *User, db *Database, order *Order) error {
    // Check user's order history
    count, err := db.GetUserOrderCount(ctx, user.ID)
    if err != nil {
        return err
    }
    
    if count >= 100 {
        return errors.New("user has reached order limit")
    }
    
    // Validate order total
    if order.Total > user.CreditLimit {
        return errors.New("order exceeds credit limit")
    }
    
    return nil
}

ctx, err := ctxdep.NewDependencyContextWithValidation(parentCtx,
    user,
    db,
    order,
    ctxdep.Validate(validateOrderLimit),
)
```

### Validation with adapters

Validators work seamlessly with adapted functions:

```go
type OrderValidator func(ctx context.Context, orderID string) error

func validateOrderExists(ctx context.Context, db *Database, orderID string) error {
    exists, err := db.OrderExists(ctx, orderID)
    if err != nil {
        return err
    }
    if !exists {
        return errors.New("order not found")
    }
    return nil
}

// Validator that uses an adapted function
func validateOrder(ctx context.Context, orderID string, validator OrderValidator) error {
    return validator(ctx, orderID)
}

ctx, err := ctxdep.NewDependencyContextWithValidation(parentCtx,
    db,
    orderID,
    ctxdep.Adapt[OrderValidator](validateOrderExists),
    ctxdep.Validate(validateOrder),
)
```

### Important notes about validation

1. **Validation runs after dependency setup**: All dependencies are initialized and adapters are processed before validators run.
2. **Validation runs before immediate dependencies**: Validators run before any immediate generator execution.
3. **First error stops validation**: If a validator returns an error, subsequent validators are not run.
4. **Backward compatibility**: The original `NewDependencyContext` still panics on validation errors, maintaining backward compatibility.
5. **Validators are not dependencies**: After validation completes, validators are removed from the context and cannot be retrieved with `Get()`.

### When to use validation

Validation is particularly useful for:

- **Request validation**: Validate incoming API requests before processing
- **Business rule enforcement**: Ensure business invariants are maintained
- **Configuration validation**: Verify configuration values meet requirements
- **State validation**: Check that the system is in a valid state before proceeding

```go
// Example: API handler with validation
func CreateOrderHandler(w http.ResponseWriter, r *http.Request) {
    var orderRequest CreateOrderRequest
    json.NewDecoder(r.Body).Decode(&orderRequest)
    
    ctx, err := ctxdep.NewDependencyContextWithValidation(r.Context(),
        &orderRequest,
        ctxdep.Get[*User](r.Context()),
        ctxdep.Get[*Database](r.Context()),
        ctxdep.Validate(validateOrderRequest),
        ctxdep.Validate(validateUserCanOrder),
        ctxdep.Validate(validateInventory),
    )
    
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    
    // Process the valid order...
}
```

## Testing with Adapters

One of the most powerful applications of adapters is in testing. Let's look at a real-world example that shows how adapters solve common testing challenges.

### The Problem: Testing Database-Dependent Code

Consider a typical business function that needs to fetch user permissions:

```Go
// Traditional approach - hard to test
type PermissionService struct {
    db *sql.DB
}

func (s *PermissionService) CheckUserPermission(userID, resource string) (bool, error) {
    var hasPermission bool
    err := s.db.QueryRow(
        "SELECT EXISTS(SELECT 1 FROM permissions WHERE user_id = ? AND resource = ?)",
        userID, resource,
    ).Scan(&hasPermission)
    return hasPermission, err
}

func (s *PermissionService) CanUserEditDocument(userID, documentID string) (bool, error) {
    // This is what we want to test, but it's tightly coupled to the database
    doc, err := s.getDocument(documentID)
    if err != nil {
        return false, err
    }
    
    if doc.OwnerID == userID {
        return true, nil
    }
    
    return s.CheckUserPermission(userID, "documents:edit")
}
```

Testing `CanUserEditDocument` traditionally requires either:
1. **A real database** - slow, requires setup/teardown, can't run in parallel
2. **Mocking frameworks** - verbose, brittle, requires interfaces for everything
3. **Global state** - makes parallel testing impossible, hidden dependencies

### The Solution: Adapters for Clean Testing

With adapters, we can cleanly separate the database access from the business logic:

```Go
// Define our adapter types
type PermissionChecker func(ctx context.Context, userID, resource string) (bool, error)
type DocumentGetter func(ctx context.Context, documentID string) (*Document, error)

// Implement the actual database functions
func checkPermissionDB(ctx context.Context, db *sql.DB, userID, resource string) (bool, error) {
    var hasPermission bool
    err := db.QueryRow(
        "SELECT EXISTS(SELECT 1 FROM permissions WHERE user_id = ? AND resource = ?)",
        userID, resource,
    ).Scan(&hasPermission)
    return hasPermission, err
}

func getDocumentDB(ctx context.Context, db *sql.DB, documentID string) (*Document, error) {
    // ... database query to get document
}

// Our business logic now uses adapters instead of direct DB access
func CanUserEditDocument(ctx context.Context, userID, documentID string) (bool, error) {
    // Get our adapters from the context
    checkPermission := ctxdep.Get[PermissionChecker](ctx)
    getDocument := ctxdep.Get[DocumentGetter](ctx)
    
    // Business logic remains the same, but now it's decoupled
    doc, err := getDocument(ctx, documentID)
    if err != nil {
        return false, err
    }
    
    if doc.OwnerID == userID {
        return true, nil
    }
    
    return checkPermission(ctx, userID, "documents:edit")
}

// In production, set up with real database
func setupProduction(db *sql.DB) context.Context {
    ctx := context.Background()
    return ctxdep.NewDependencyContext(ctx,
        db,
        ctxdep.Adapt[PermissionChecker](checkPermissionDB),
        ctxdep.Adapt[DocumentGetter](getDocumentDB),
    )
}
```

### Testing Becomes Trivial

Now testing is clean and doesn't require any mocking frameworks:

```Go
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
                // No database needed - just return what we need for the test
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
                    if userID == "user789" && resource == "documents:edit" {
                        return true, nil
                    }
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
            
            if tt.expectedError && err == nil {
                t.Error("expected error but got none")
            }
            if !tt.expectedError && err != nil {
                t.Errorf("unexpected error: %v", err)
            }
            if allowed != tt.expectedAllow {
                t.Errorf("expected allowed=%v but got %v", tt.expectedAllow, allowed)
            }
        })
    }
}
```

### Key Advantages

1. **No Mocking Frameworks** - Just simple functions that return what you need
2. **Parallel Testing** - Each test has its own context, no shared state
3. **Clear Test Intent** - The test setup clearly shows what each dependency will do
4. **Type Safety** - The compiler ensures your test adapters match the expected signatures
5. **No Abstraction Breaking** - The business logic doesn't know it's being tested
6. **Lazy Evaluation** - Database connections are only made if actually needed

Compare this to traditional approaches:
- **With interfaces everywhere**: Requires changing production code structure for testing
- **With mocking frameworks**: Verbose setup, runtime reflection, less clear intent
- **With global variables**: Can't run tests in parallel, hidden dependencies

The adapter pattern preserves the natural structure of your code while making it completely testable.

### Cleanup behavior

- Cleanup must be explicitly enabled with `WithCleanup()` or `WithCleanupFunc()`
- Cleanup is performed by explicitly calling `Cleanup()` on the returned dependency context, typically with `defer`
- Cleanup functions are called exactly once, even if `Cleanup()` is called multiple times on the same context
- Cleanup happens synchronously when `Cleanup()` is called
- Dependencies created by generators are also cleaned up
- Each context manages its own dependencies - calling `Cleanup()` on a child context doesn't affect the parent
- Custom cleanup functions take precedence over automatic `io.Closer` cleanup
- Using `WithCleanupFunc()` automatically enables cleanup for all dependencies

This explicit cleanup design gives you full control over when resources are released, preventing race conditions that could occur with automatic cleanup based on context cancellation.

# Why all this is important

Testing.

Testing is what started the idea for this.

Without a system like this, overriding dependencies in a unit test can be awkward.

Consider a setup like this:

```Go
module user

func GetUserData(userId string) *UserData {...}

module security

func isPermitted(request *Request) bool {
    userData := user.GetUserData(request.userId)
    if userData.isAdmin {
        return true
    }
    // other stuff...
    return false
}
```

This 100% works in a production setting. The issue is that it's hard to override the call to `GetUserData` to test the rest of the function.

A different approach would be to abstract the `*UserDataService` into an interface and provide a default implementation in a global variable. That works fine, until it doesn't.

```Go
module user

type UserFetcher interface {
    GetUserData(userId string) *UserData
}

type DefaultUserFetcher struct {}
func (d *DefaultUserFetcher) GetUserData(userId string) *UserData {...}

var Fetcher UserFetcher = &DefaultUserFetcher{}

module security

func isPermitted(request *Request) bool {
    userData := user.Fetcher.GetUserData(request.userId)
    if userData.isAdmin {
        return true
    }
    // other stuff...
    return false
}
```

If you are running unit tests in parallel, you have subtle ordering and race conditions as you are running the tests and the state of the global pointer can change in unpredictable ways. We've run into this in the past and it's quite aggravating.

Another approach would be to add the `*UserData` to the context with `context.WithValue()`. This library is riffing off of that same concept.

With context dependencies, the unit test for this is easy:

```Go
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
        Id:      42, 
        Name:    "George Burgyan",
        IsAdmin: true,
    }
    dc := ctxdep.NewDependencyContext(context.Background(), user)
    permitted := isPermitted(dc)
    assert.True(t, permitted)    
}
```

While in this trivial example you would pass a `*UserData` to the function under test, it starts getting tricky once you have more layers of functions between the creation of an object and its user. This tends to make the function signatures grow larger and larger as more things need to get passed.

Even if you never use the generators, this is a key advantage to unit testing your code.

# Special cases

## Cyclic dependencies

While processing a generator, that generator can request an additional object from the dependency context. This can happen either through parameters that are passed to the generator directly or through requesting them explicitly from the dependency context. There are provisions in the library to check for circular dependencies. In case such a circular dependency is encountered, an error is returned.

If this check were not included, then a circular dependency would lead to a deadlock due to the checks that ensure thread safety.

## Thread safety

All efforts have been made to ensure that any accesses to the dependency context are done in a way that is thread safe. Additionally, if two goroutines try to invoke a generator simultaneously, one will block temporarily and the generator function will only be executed once.

The intent is that a generator may involve potentially expensive operations, so it would be wasteful to invoke it multiple times.

This same mechanism is also used when resolving immediate dependencies to block the requester while the generator runs.

## Debugging using `Status`

A call to `ctxdep.Status(ctx)` will return a string representation of everything in the dependency context. This can be used to verify what is and is not in the context in case something unexpected occurs.

A great example of what `Status` returns is in the `Test_ComplicatedStatus` test:

```Go
// Set up a parent context that returns a concrete implementation of an interface
c1 := NewDependencyContext(context.Background(), func() *testImpl {
        return &testImpl{val: 42}
    }, func() *testDoodad {
        return &testDoodad{val: "wo0t"}
    })

// Make another status from that one
c2 := NewDependencyContext(c1, func(in testInterface) *testWidget {
return &testWidget{val: in.getVal()}
}, &testDoodad{val: "something cool"})

widget := Get[*testWidget](c2)
```

A call to `Status(c2)` after execution returns:

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

We can dissect this line by line:

* `*ctxdep.testDoodad - direct value set` is the simplest case. This is a simple dependency that has been set.
* `*ctxdep.testWidget - created from generator: (ctxdep.testInterface) *ctxdep.testWidget` notes that the `*testWidget` was created by calling a generator that takes a `testInterface`  that return the widget.
* `ctxdep.testInterface - imported from parent context` says that the previous call's input dependency was fulfilled from the parent context's value. These imports are an optimization.
* `parent dependency context` shows a navigation to this context's parent.
* `*ctxdep.testDoodad - uninitialized - generator: () *ctxdep.testDoodad` is a generator that hasn't yet been run.
* `*ctxdep.testImpl - created from generator: () *ctxdep.testImpl` shows that the `*testImpl` was created by calling a generator.
* `ctxdep.testInterface - assigned from *ctxdep.testImpl` states that the `testInterface` was made by casting the `*testImpl` to the interface because it implements all of the functions of the interface.


## Handling errors

The above examples use the `Get()` method to retrieve things from the context. The expectation in general is that anything that is requested **will** in the context. If it's not, the behavior of `Get()` is to `panic`. This simplifies the usage because you don't have to do error checks everywhere.

If you *do* want to handle errors, you can call the `GetWithError()` function that works in exactly the same way as the regular `Get()`, but will also return errors if the type requested is not found. If a generator with an error is invoked, the error from the generator will be returned.

In case errors are returned, they will be of type `ctxdep.DependencyError`. The status of the context will be in that error object at time of evaluation to aid in any debugging that is needed.

Note, however, that this will still `panic` if the dependency context is not found. This is intentional as it grossly violates the preconditions for the call. A `panic` from a generator will still leak out as well.

## Optional dependencies

For cases where dependencies might not be present and you want to handle this gracefully without panicking, you can use the `GetOptional()` function:

```Go
func processWithOptionalCache(ctx context.Context) {
    cache, found := ctxdep.GetOptional[*CacheService](ctx)
    if found {
        // Use cache for faster processing
        result := cache.Get("key")
        if result != nil {
            return result
        }
    }
    // Fall back to slower processing without cache
    return computeExpensiveResult()
}
```

This is particularly useful for:
- Feature flags and optional services
- Graceful degradation when certain services are unavailable
- Testing scenarios where you want to test with and without certain dependencies

There's also `GetBatchOptional()` which returns a slice of booleans indicating which dependencies were successfully found:

```Go
var cache *CacheService
var logger *LogService
var metrics *MetricsService

results := ctxdep.GetBatchOptional(ctx, &cache, &logger, &metrics)
// results[0] indicates if cache was found
// results[1] indicates if logger was found
// results[2] indicates if metrics was found
```

## Getting multiple values from the context

If you need multiple values from the dependency context, there is a `GetBatch()` and `GetBatchWithError()` where you can pass multiple pointers in to, and they will be filled in from the context:

```Go
func f(ctx context.Context) {
    var widget *Widget
    var doodad *Doodad
    ctxdep.GetBatch(&widget, &doodad)
}
```

The semantics of the calls are identical to the regular `Get()` and `GetWithError()` except you can get multiple values at once. This is a very slight optimization time-wise as it only looks up the dependency context from the context once.

If there is demand, functions like `Get2()`, `Get3()`, etc. can be added.

## Dependency checking when adding generators

Any time dependencies are added, the state of the context is validated. If there is a generator that has an input parameter that is not fulfilled by the contents of the context, the add immediately panics.

The general case is that you know the types of everything that is put into the context. Certainly the dependency context knows what has been put inside it. When a generator is added to the context, we can verify that when it's invoked, that we can satisfy its parameters.

This is done to prevent cases where there may be some rare use case that only infrequently gets triggered. Since we can immediately tell there will be an error if it's invoked, report on this early to prevent errors that may be hard to track down in production.

## Multiple dependency contexts in the context

It is valid to have multiple dependency contexts on the context stack. An easy example would be to have service-level objects that are added at startup to one, then a request level dependency context added for each request. Instead of having an explicit scope management system built in, the context keeps track of all of that for us.

When looking for a dependency, either directly or to fulfil the requirements for a generator, the current dependency context is checked. If it's not found, any dependency contexts that also exist on the context are also checked. This also applies to checking for the existence of dependencies when adding generators.

A key point to note is that you cannot have a lower level (e.g. service level dependency context) depend on a higher level (e.g. request) dependency. Since the higher-level dependency can change with requests, it would make the dependency caching at the lower level invalid. This is enforced by checking for dependencies when adding generators. This structurally prevents having defective dependency contexts set up.

## Multiple types assignable to the same target

This is an edge case that is _not_ handled. If a type is requested but is not present in the dependency context, and there are multiple types in the context that are assignable to the requested type, one of the types in the context will be used. Which one is not defined. This is typically manifested by having multiple types implementing the same interface.

## Strict vs. loose construction of contexts

The default behaviour of the context dependencies is that if multiple dependencies are present, either for concrete values or generators, the construction of the context will `panic`. This is to follow the "fail fast" mindset since there likely is a bug in specifying what is going to be in the context. This will surface that issue quickly.

While this is generally fine for production code, it can cause annoyance when writing tests. There are cases where you have a default set of common dependencies, but for *this test* you need to have something else to test a use case. The `WithOverrides()` option is provided to account for this.

```Go
// This will panic because two values fill the same slot
ctx := ctxdep.NewDependencyContext(ctx, widgetA, widgetB)

// This allows overriding - widgetB will be used
ctx := ctxdep.NewDependencyContext(ctx, ctxdep.WithOverrides(), widgetA, widgetB)

// Options and dependencies can be mixed in any order
ctx := ctxdep.NewDependencyContext(ctx, widgetA, ctxdep.WithOverrides(), widgetB)

// Multiple options can be combined
ctx := ctxdep.NewDependencyContext(ctx, 
    ctxdep.WithOverrides(),
    ctxdep.WithCleanup(),
    widgetA, 
    widgetB,
)
```

When constructing a context with `WithOverrides()`, you can freely override concrete values and generators; the last one added will be used. In case that there are both generators and concrete values, the last value will be used; a generator will never override a value.

For backward compatibility, `NewLooseDependencyContext` is still available but is deprecated in favor of using `NewDependencyContext` with `WithOverrides()`.

## Overriding the parent context

In certain cases you need to reuse a parent context because whatever created the context you have did not properly copy the context. We've encountered this with gRPC services having a parent context of `context.Background()` on goroutines that are created to service requests. If you pass a context as the first dependency parameter when you `NewDependencyContext`, you can override where parent dependencies are looked up. Note that this only works when you pass the context as the first real parameter to `NewDependencyContext`. This works even if the first real parameter is inside a slice that has been passed in at initialization.

## Special handling of contexts

Any generators that are run will run from the context from which they were created. What this means is that there is no chance for a child dependency ever filling a requirement with a parent's dependency's generator.

This is an important security feature. If a child dependency could fill a requirement for a parent's generator, data from one part of the code could pollute elsewhere because the results of the generators are saved for later use. This can also be a potential security vulnerability where the wrong data could potentially be used.

There is special handling of the caller's context such that the deadlines and everything that comes from the context are still honored. If the caller's context times out, then a generator that respects the timeouts will properly abort. The result of that error is not cached.

## Timing

There is the ability for the context dependencies to use the sister library, `go-timing`, to keep track of the execution time during runtime. Please refer to the [documentation for that library](https://github.com/gburgyan/go-timing) for full details on its usage.

By default, this integration is disabled as there is a very minor performance penalty (generally well under a microsecond per generator invocation).

This can be enabled by setting `ctxdep.TimingMode`:

* `TimingDisable` - the default off state.
* `TimingImmediate` - create a placeholder timing context for the immediate dependency processing.
* `TimingGenerators` - in addition to creating a context for the immediate processing, will also create a timing context whenever a generator is going to be invoked.

For generators, this also has the ability to note when a generator was waiting for another goroutine to finish the generator.

For instance, the test `Test_ImmediateDependency_LongCall` will show something like this:

```text
[ImmediateDeps] > CtxGen(*ctxdep.testWidget) - 101.05975ms (generator:() *ctxdep.testWidget)
CtxGen(*ctxdep.testWidget) - 49.9845ms (generator:() *ctxdep.testWidget, wait:parallel)
```

This indicates that the immediate generator for the `*testWidget` took roughly 100ms to execute. In the test code, there is a call to fetch the same dependency delayed by 50ms. This will block until the completion of the call by the generator. In this case, the main goroutine was blocked by the same generator that was previously started by the immediate processing. Since this is still time that is being consumed, even though it's simply a wait, this will be noted by the additional detail of "wait:parallel" in the timing context.

The `[ImmediateDeps]` is configured as an async timing context with no time allotted to itself so it will not affect the overall timings.

An example of the `TimingGenerators` can be found in the `Test_MultiLevelDependencies` test. The output of this looks like:

```text
CtxGen(*ctxdep.testDoodad) - 30.083µs (generator:(context.Context, *ctxdep.testWidget) *ctxdep.testDoodad)
CtxGen(*ctxdep.testDoodad) > CtxGen(*ctxdep.testWidget) - 3.084µs (generator:(context.Context) *ctxdep.testWidget)
```

In this case, this is showing that the call to get the `*testDoodad` invoked a generator `gen(context.Context, *ctxdep.testWidget) *ctxdep.testDoodad`, which needed the `*testWidget`, which invoked another generator. The names of the timing contexts are that of the requested type that prompted the generator call. The actual signature of the generator is added as additional details for the timing context. If a generator produces multiple outputs, only the first call to the context dependencies gets explicit timing logging as the generator is only invoked once.

This level of detail may or may not be helpful, but it does add a lot of extra information to the timing that is being gathered which can be useful if you are completely stumped about how things are working or time is being spent.

What may be more useful generally is to use `TimingImmediate` and handle any known long calls with your own timing calls.

# License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.