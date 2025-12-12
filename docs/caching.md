# Caching

The dependency context can cache generator results across requests using an external cache provider. This is useful for expensive operations that don't change frequently.

## When to Use Caching

Caching is appropriate when:
- Generator results are expensive to compute (DB queries, API calls)
- Results don't change frequently
- Results can be shared across multiple requests/contexts

**Note:** Results are already cached within a single context. This caching feature is for sharing results *across* contexts.

## Basic Caching

Wrap a generator with `Cached()`:

```go
var cache = NewYourCacheType()

func UserDataGenerator(ctx context.Context, userService UserDataService, request *Request) (*UserData, error) {
    return userService.Lookup(request)
}

func HandleRequest(ctx context.Context, request *Request) *Response {
    ctx = ctxdep.NewDependencyContext(ctx,
        &UserDataServiceImpl{},
        request,
        ctxdep.Cached(cache, UserDataGenerator, time.Minute*15),
    )
    // ...
}
```

The generator result will be cached for 15 minutes.

## The Cache Interface

Your cache must implement:

```go
type Cache interface {
    Get(ctx context.Context, key string) []any
    SetTTL(ctx context.Context, key string, value []any, ttl time.Duration)
}
```

- `Get` returns `nil` if the key is not found
- `SetTTL` stores the value with the given time-to-live
- The cache may evict entries before TTL expires (that's fine)

**Important:** The `[]any` returned by `Get` must be equivalent to what was passed to `SetTTL`.

### Simple In-Memory Implementation

```go
type SimpleCache struct {
    data sync.Map
}

type cacheEntry struct {
    value   []any
    expires time.Time
}

func (c *SimpleCache) Get(ctx context.Context, key string) []any {
    if v, ok := c.data.Load(key); ok {
        entry := v.(*cacheEntry)
        if time.Now().Before(entry.expires) {
            return entry.value
        }
        c.data.Delete(key)
    }
    return nil
}

func (c *SimpleCache) SetTTL(ctx context.Context, key string, value []any, ttl time.Duration) {
    c.data.Store(key, &cacheEntry{
        value:   value,
        expires: time.Now().Add(ttl),
    })
}
```

## Cache Key Generation

The inputs to the generator must be convertible to cache keys. The library tries these approaches in order:

### 1. Keyable Interface (Recommended)

```go
type Keyable interface {
    CacheKey() string
}

func (r *Request) CacheKey() string {
    return fmt.Sprintf("request:%d", r.ID)
}

func (u *UserDataService) CacheKey() string {
    return fmt.Sprintf("service:%s", u.Endpoint)
}
```

### 2. RegisterCacheKeyProvider

For types you don't control:

```go
func init() {
    ctxdep.RegisterCacheKeyProvider(func(r *ExternalRequest) string {
        return fmt.Sprintf("ext:%s", r.ID)
    })
}
```

### 3. Stringer Interface

If the type implements `fmt.Stringer`, that's used:

```go
func (r *Request) String() string {
    return fmt.Sprintf("Request{ID:%d}", r.ID)
}
```

### 4. JSON Serialization (Fallback)

As a last resort, the object is JSON-serialized and that's used as the key.

## Advanced Options with CachedOpts

For more control, use `CachedOpts()`:

```go
ctx := ctxdep.NewDependencyContext(ctx,
    ctxdep.CachedOpts(ctxdep.CtxCacheOptions{
        Cache:             cache,
        Generator:         UserDataGenerator,
        CacheTTL:          time.Minute * 15,
        RefreshPercentage: 0.75,
    }),
)
```

### RefreshPercentage

Triggers background refresh before TTL expires:

```go
CachedOpts(CtxCacheOptions{
    Cache:             cache,
    Generator:         gen,
    CacheTTL:          time.Minute * 10,
    RefreshPercentage: 0.75,  // Refresh after 7.5 minutes
})
```

When access occurs after 75% of TTL has elapsed:
1. The cached value is returned immediately
2. A background goroutine refreshes the cache
3. Only one refresh happens even with concurrent requests

### DurationProvider

Dynamic TTL based on the result:

```go
CachedOpts(CtxCacheOptions{
    Cache:     cache,
    Generator: gen,
    DurationProvider: func(result *UserData) time.Duration {
        if result.IsPremium {
            return time.Hour  // Cache premium users longer
        }
        return time.Minute * 5
    },
})
```

### CacheTTL Interface

The result type can control its own TTL:

```go
type CacheTTL interface {
    CacheTTL() time.Duration
}

func (u *UserData) CacheTTL() time.Duration {
    if u.IsPremium {
        return time.Hour
    }
    return time.Minute * 5
}
```

This is checked if no `DurationProvider` is specified.

## Internal Locking

The caching system includes internal locking to ensure:
- Only one goroutine runs the generator for a given key
- Concurrent requests wait for the first to complete
- No duplicate generation occurs

**Note:** This is local locking only. For distributed systems, see below.

## Distributed Caching

For distributed caching with proper locking, see the [go-rediscache](https://github.com/gburgyan/go-rediscache) package. It provides:
- Redis-backed caching
- Distributed locking
- Automatic key serialization

---

## See Also

- [Generators](generators.md) - How generators work
- [Advanced](advanced.md) - Performance considerations
