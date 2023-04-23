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

# Usage

## Basic usage

The simplest case is to put an object into the dependency context, then later on pull it out:

```Go
type MyData struct {
    value string
}

func Processor(ctx context.Context) {
    ctx = ctxdep.NewDependencyContext(ctx, &MyData{data:"for later"})
    client(ctx)
}

func client(ctx context.Context) {
    data := ctxdep.Get[*MyData](ctx)
    fmt.Printf("Here's the data: %s", data.value)
}
```

This works very similarly to how the base `context.WithValue()` system works: you add something to the context, pass it around to functions you call, and you pull things out of it.

A key point is that the client code above *never* changes in how it works. Fundamental to the design is that you always ask for an object out of the context, and you receive it--it doesn't matter how that object got into the context, it just works. There are a couple of ways of doing this operation, but it is always the same in concept.

## Slices

A slice of values can be passed in to the dependencies. If a `[]any` is passed, those are flattened and evaluated as if they weren't in a sub-slice. This is to support a use case where several components return `[]any` for their dependencies. This is simply a helper to prevent having to manually concatenate slices before passing them to `NewDependencyContext`.

```Go

func componentADeps() []any {
	return []any{ /* objects and generators */}
}

func componentBDeps() []any {
    return []any{ /* objects and generators */}
}

func Processor(ctx context.Context) {
    ctx = ctxdep.NewDependencyContext(ctx, componentADeps(), componentBDeps())
    client(ctx)
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
    ctx = ctxdep.NewDependencyContext(ctx, &ServiceCaller{})
    client(ctx)
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
```

This will use the parameters that are passed to the generator to generate a key for the cache. The cache will then be used to store the results of the generator.

The `cache` object is expected to implement the `ctxdep.Cache` interface. The `ctxdep.Cache` interface is:

```go
type Cache interface {
    Get(key string) []any
    SetTTL(key string, value []any, ttl time.Duration)
    Lock(key string) func()
}
```

The expectation is that this interface can wrap whatever caching system you want to use. Lock is used to ensure that only one goroutine is generating the value for a key at a time, however the implementation of that is not required -- you can simply return a no-op function or nil and things will function as expected. 

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
    ctx := ctxdep.NewDependencyContext(contest.Background(), user)
    permitted := isPermitted(ctx)
    assert.True(t, permitted)    
}
```

While in this trivial example you would pass a `*UserData` to the function under test, it starts getting tricky once you have more layers of functions between the creation of an object and its user. This tends to make the function signatures grow larger and larger as more things need to get passed.

Even if you never use the generators, this is a key advantage to unit testing your code.

# Special cases

## Cyclic dependencies

While processing a generator, that generator can request an additional object from the dependency context. This can happen either through parameters that are passed to the generator directly or through requesting them explicitly from the dependency context. There are provisions in the library to check for circular dependencies. In case such a circular dependency is encountered, an error is returned.

If this check were not included then a circular dependency would lead to a deadlock due to the checks that ensure thread safety.

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

Thile this is generally fine for production code, but it can cause annoyance when writing tests. There are cases where you have a default set of common dependencies, but for *this test* you need to have something else to test a use case. The `NewLooseDependencyContext` is provided to account for this.

When constructing a context "loosely," you can freely override concrete values and generators; the last one added will be used. In case that there are both generators and concrete values, the last value will be used; a generator will never override a value.

## Special handling of contexts

Any generators that are run will run from the context from which they were created. What this means is that there is no chance for a child dependency ever filling a requirement with a parent's dependency's generator.

This is an important security feature. If a child dependency could fill a requirement for a parent's generator, data from one part of the code could pollute elsewhere because the results of the generators are saved for later use. This can also be a potential security vulnerability where the wrong data could potentially be used.

There is special handling of the caller's context such that the deadlines and everything that comes from the context are still honored. If the caller's context times out, then a generator that respects the timeouts will properly abort. The result of that error is not cached.