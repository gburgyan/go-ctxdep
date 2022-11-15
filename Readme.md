![Build status](https://github.com/gburgyan/go-ctxdep/actions/workflows/go.yml/badge.svg) [![PkgGoDev](https://pkg.go.dev/badge/github.com/gburgyan/go-ctxdep)](https://pkg.go.dev/github.com/gburgyan/go-ctxdep)

# Go Context Dependencies

## Installation

```bash
go get github.com/gburgyan/go-ctxdep
```

## Features

Go already has a nice way to keep track of things in your context: `context.Context`. This adds some helpers to that to simplify getting things out of that context that is already being passed around.

The fundamental as

The basic feature of the go-ctxdep library is to provide a simple way to access needed dependencies away from the creation of those dependencies. It provides ways of putting dependencies, which are simply instances of objects that can be pulled out later, into the context that is already there. For objects that may be more costly to generate dependencies can be represented by generators that are called when they are first referenced. In cases where you know an expensive dependency is needed, you can mark the generator to run immediately which will fire off the generator in a background goroutine to be able to give it as much of a head start as possible.

All this is done so the calling code is as simple and intuitive as possible and relying on as little magic as practical. There is no configuration to go wrong and everything is in code to clearly show the intent of the users of this class.

## Usage

It's designed to be simple and intuitive to use both for real code and for unit tests. In the most basic form this can be as simple as:

```Go
type ImportantData struct {
    value string
}

func main() {
    ctx := context.Background()
    ctx = ctxdep.NewDependencyContext(ctx, &ImportantData{data:"for later"})
    doStuff(ctx)	
}

func doStuff(ctx context.Context) {
    data := ctxdep.Get[*ImportantData](ctx)
    fmt.Printf("Here's the data: %s", data.value)
}
```

The call to `ctxdep.Get` is passed pointer to one or more types that you want to get from the context. Those types are simply pulled from the dependency context and the variable is filled in with no muss or fuss. This is further simplified by the default behaviour of `Get` where it just panics if the required dependency is not there. You can call `GetWithError` if you want to handle various errors yourself.

Alternately, if you want to run a test of the `doStuff` function with unique non-production data, it would be easy to simply add a different `ImportantData` to the context dependencies.

### Types of dependencies

There are two types of dependencies supported by this library: direct dependencies and generator-based dependencies. This allows for flexibility with both production workloads and a suitable hook to allow for easily writing unit tests.

#### Direct Dependencies

An example of direct dependencies are in the example above -- it's simply an instance that can be fetched from the dependency context. 

#### Generator Dependencies

In addition to simple direct dependencies we support generator dependencies. A generator dependency is a function that can be used to create the required dependency. Generators take the form:

```Go
func generator(ctx Context.context, param InputType) (result1 Type1, result2 Type2, error)
```

A generator:

* May have a `context` parameter
* May have additional parameters that will be fulfilled by looking up the dependency from the dependency context
* Must return at least one non-error object
* Return zero or one `error` type

Once a generator is added to the dependency context, if any of the result types are requested by `GetDependencies` then the generator will be called. If a generator returns more than one result type, then all returned types are added to the dependency context.

If a generator has non-`context` parameters, the dependency engine will look up those types from the dependency context. This can easily go through several levels if one generator requests a parameter that another generator produces.

### Differences from global variables and global functions?

A common pattern in any service, Golang included, is to have the facades of services that are called as a package. This is convenient, but it leads to difficulty in testing the client code that relies on the service as there is no good way to intercept calls to a package that makes service calls.

One way around this is to introduce an interface and a default implementation of that interface and a global reference to an instance of that object. This generally works but can lead to fragility when running parallel tests, each of which may want to override the functionality in a different way. This also breaks down in cases when you simply need something that is dependent on the context of the caller; you can't store the current user in a global variable as it's shared between all concurrent requests.

### Immediate dependencies

A special case of a generator-based dependency is that of an immediate dependency. An immediate dependency is one in which the generator for the value is called immediately in a new goroutine. This allows for dependencies that take some time to compute, but are always (or nearly) accessed during the course of processing. By starting the generator processing early it gives a head start to the generator. Since it's running in the background from the start of the request it decouples the request for the dependency from the call itself.

To create an immediate dependency, simply wrap the generators with a call to `Immediate`.

```Go
type UserData struct{}

func FetchUserData(ctx context.Context) *UserData {
    // some longer processing to get user's data
    return &UserData{}
} 

func HandleRequest(ctx context.Context, request Request) Response {
    ctx = ctxdep.NewDependencyContext(ctx, Immediate(FetchUserData))
    process(ctx)
}

func process(ctx context.Context)
    // The call to FetchUserData was started at the time the DependencyContext
    // was instanciated.
    user := ctxDep.Get[*UserData](ctx) 
}
```

## Common use cases

### Fetch configuration based on request

A common thing that occurs in many services during processing is accessing some configuration about a caller, user information for instance. Everything required to get this information is available at or near the point that a request arrives at a service. This presents several challenges:

* Should any data that is needed be looked up prior to the processing?
  * This may lead to wasted work looking things up that may not be used on every request
* Where should that information be stored?
* Look it up at the time it's needed?
  * How is the information that is critical to a request carried to the called function?
  * If there are multiple users of this data, how is caching of that data handled?

This library solves this by introducing generator functions as part of the context dependencies.

```Go
type UserData struct {
    Id      int
    Name    string
    IsAdmin bool
}

func UserDataGenerator(request Request) func(context.Context) UserData {
    return func(ctx context.Context) (*UserData, error) {
        return UserDataService.Lookup(request)
    }
}

func HandleRequest(ctx context.Context, request Request) Response {
    ctx = ctxdep.NewDependencyContext(ctx, UserDataGenerator(request))
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

This decouples the client code from worrying about how the `UserData` is created--it's just there. The other advantage is that the call to look up the user's data is not made until it is needed. In the trivial example about this does not save any time since it's always used, but in many cases not everything that _can_ be used is used.

### Unit testing

This leads into unit testing the above code. If you wanted to write tests against the `isPermitted` function above normally, you would have to do one of:

* Pass in the `UserData` to the function so the test code can supply different test data
* Create a mock `UserDataService` and use that instead of a real one to look up the data
* Create a mock object of what the `UserDataService` receives to produce data at that level

Of these, only the first one is good enough. Everything else relies on some variety of global variable to allow substitution of data elsewhere.

The downside of the first option is that it makes the signatures of functions increasing large, and only moves the problem upstream.

What we can do instead is something like this:

```Go
func Test_isPermitted_TestAdmin(t *testing.T) {
    ctx := ctxdep.NewDependencyContext(context.Background(), &UserData{
        Id:      42, 
        Name:    "George Burgyan",
        IsAdmin: true,
    })
    permitted := isPermitted(ctx)
    assert.True(t, permitted)
}
```

Even though in the production code path the user is filled in with a generator, the testing code can easily add new test data using the direct dependency technique.

Additionally, even if the production code uses a generator to fulfil the requested object, a test can just as easily use a direct dependency for that object.

### Access to a service facade

Another common use case is to have a service facade that is typically accessed by a global package. By moving the facade to the dependency context we can use a test version of it as long as it's implements the same interface:

```Go
type Stuff struct {}

type ServiceAccessor interface {
    GetStuff(id string) Stuff
}

type RealService struct {
}

func (s *RealService) GetStuff(id string) *Stuff {
    // call a service
    return &Stuff{}
}
```

The normal code path can do something like:

```Go
func main() {
    ctx := context.Background()
    ctx = ctxdep.NewDependencyContext(ctx, &RealService{})
    client(ctx)
}

func client(ctx context.Context) {
    service := ctxdep.Get[*ServiceAccessor](ctx)
    stuff := service.GetStuff("key")
}
```

This already is a nice looking function. This is also easy to test now!

```Go
type mockService struct {
    result Stuff
}

func (s *mockService) GetStuff(id string) Stuff {
    return s.result
}

func Test_client(t *testing.T) {
    ctx := ctxdep.NewDependencyContext(context.Background(), &mockService{
        result: Stuff{}
    })

    client(ctx)
}
```

The same code path now fetches the mock service since it implements the same interface as the real service facade.

## Special cases

### Cyclic dependencies

While processing a generator, that generator can request an additional object from the dependency context. This can happen either through parameters that are passed to the generator directly or through requesting them explicitly from the dependency context. There are provisions in the library to check for circular dependencies. In case such a circular dependency is encountered, an error is returned.

If this check were not included then a circular dependency would lead to a deadlock due to the checks that ensure thread safety.

### Thread safety

All efforts have been made to ensure that any accesses to the dependency context are done in a way that is thread safe. Additionally, if two goroutines try to invoke a generator simultaneously, one will block temporarily and the generator function will only be executed once.

The intent is that a generator may involve potentially expensive operations, so it would be wasteful to invoke it multiple times.

This same mechanism is also used when resolving immediate dependencies to prevent running the same generator more than once.
