package ctxdep

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Keyable is an interface that can be implemented by a
// dependency to provide a unique key that can be used to cache the
// result of the dependency. Implementing this interface is required
// if you want to use the Cached() function.
type Keyable interface {
	// CacheKey returns a key that can be used to cache the result of a
	// dependency. The key must be unique for the given dependency.
	// The intent is that the results of calling generators based on the
	// value represented by this key will be invariant if the key is
	// the same.
	CacheKey() string
}

var cacheKeyProviders = make(map[reflect.Type]func(any) string)

// RegisterCacheKeyProvider registers a function that can be used to
// generate a cache key for a given type. This is used by the Cached()
// function to generate a cache key for the given dependency. This
// allows for the cache key to be generated for types that do not
// implement the Keyable interface.
func RegisterCacheKeyProvider(t reflect.Type, f func(any) string) {
	cacheKeyProviders[t] = f
}

// Cache is an interface for a cache that can be used with the Cached() function.
// The cache must be safe for concurrent use. The cache is not required to
// support locking, but if it does not support locking then the generator
// function must be safe for concurrent use.
//
// Internally this saves the results of the generator function in the cache.
// While it is possible to persist the results, be aware that this may be tricky.
// The cache will be passed a slice of arbitrary pointers to the results of the
// generator function.
type Cache interface {
	// Get returns the value for the given key, or nil if the key is
	// not found.
	Get(ctx context.Context, key string) []any

	// SetTTL sets the value for the given key, and sets the TTL for
	// the key. If the TTL is 0, the key will not expire.
	// The value parameter is a slice of pointers to the results of
	// the generator function.
	SetTTL(ctx context.Context, key string, value []any, ttl time.Duration)

	// Lock locks the given key in an external store. Internally, the caching system
	// has the concept of an internal lock, but this allows the cache to lock the key
	// in an external store, such as Redis. This is used to prevent multiple goroutines
	// from calling the generator function for the same key. If the key is already locked, this
	// should block until the key is unlocked. The returned function must be called to unlock
	// the key. This is optional; if the cache does not support locking, it can return nil or
	// no-op function.
	Lock(ctx context.Context, key string) func()
}

// CtxCacheOptions contains the options for the CachedOpts function.
type CtxCacheOptions struct {
	// TTL is the time-to-live for the cache entry. If TTL is 0, the cache
	// entry will never expire, but it can still be evicted due to memory
	// pressure or other reasons.
	TTL time.Duration

	// DurationProvider is a function that is called with the results of the generator function.
	// The duration provider should return the TTL for the cache entry.
	// The parameters passed to the duration provider is a slice of the non-error
	// results of the generator function. The duration provider can use this
	// to determine the TTL.
	DurationProvider CacheDurationProvider

	// RefreshPercentage expresses the percentage of the TTL at which the cache
	// entry should be refreshed. If RefreshPercentage is 1, the cache entry will
	// not be refreshed. If RefreshPercentage is 0.5, the cache entry will be refreshed
	// halfway through its TTL. This setting is useful for ensuring that the cache
	// entry is always fresh and fetching new data before the cache entry expires.
	RefreshPercentage float64

	// ForceRefreshPercentage expresses the percentage of the TTL at which the cache
	// entry should be refreshed. If ForceRefreshPercentage is 1, the cache entry will
	// not be refreshed. If ForceRefreshPercentage is 0.5, the cache entry will be refreshed
	// halfway through its TTL. This setting is useful for ensuring that the cache
	// entry is always fresh and fetching new data before the cache entry expires. This
	// is a way of tuning when the refresh should happen. It sets an upper bound on when
	// the RefreshAlpha will no longer be relevant. If this is <= 0 or >= 1, it will be
	// ignored.
	ForceRefreshPercentage float64

	// RefreshAlpha is the alpha value used to calculate the probability of refreshing
	// the cache entry. The time range between when a cache entry is eligible for
	// refresh and the TTL-LockTTL is scaled to the range [0, 1] and called x.
	// The probability of refreshing the cache entry is calculated as x^(alpha-1).
	// If RefreshAlpha is 1 or less, the cache entry will be refreshed immediately
	// when it is eligible for refresh. A higher alpha value will make it less likely
	// that the cache entry will be refreshed.
	// A value of 0 will inherit the default alpha for the cache.
	RefreshAlpha float64

	// now is used for testing purposes to override the current time.
	now func() time.Time
}

// CacheDurationProvider is a type alias for a function that takes a slice of any type
// and returns a time.Duration. This function is used in the CachedCustom function to
// determine the Time to Live (TTL) for a cache entry. The function is called with the
// results of the generator function and should return the TTL based on these results.
// The parameters passed to the duration provider is a slice of the non-error results
// of the generator function. The duration provider can use this to determine the TTL.
type CacheDurationProvider func(CtxCacheOptions, []any) time.Duration

// CacheTTL is an interface that can be implemented by a
// dependency to provide a Time to Live (TTL) for a cache entry.
// Implementing this interface is optional, but can be used to
// control the TTL of a cache entry on a per-object basis.
// If a result object implements this interface, the CacheTTL() method
// will be called to get the TTL for the cache entry. The returned
// duration will be used as the TTL for the cache entry.
type CacheTTL interface {
	// CacheTTL returns a time.Duration that represents the Time to Live (TTL)
	// for a cache entry. The TTL is the duration that the cache entry should
	// be kept in the cache before it is considered stale and is eligible for
	// eviction. The TTL is not a guarantee that the entry will be kept in the
	// cache for the entire duration, as the cache may choose to evict the entry
	// earlier due to memory pressure or other reasons.
	CacheTTL() time.Duration
}

// Cached returns a function that caches the result of the given
// generator function. The cache key is generated by calling the
// CacheKey() method on the key parameter. The cache key must be
// unique for the given dependency and the given context. For parameters
// that do not implement the Keyable interface, the type name of the parameter
// is used as the cache key. This is to account for things that
// come from the dependency context that are purely structural. For example,
// a client connection library such as Resty.
//
// The intent is that you can easily adapt any cache to work with
// this function. For example, if you have a Redis cache, you can
// implement the Cache interface by calling the Redis commands.
// Or you can use a library like Ristretto to implement the Cache
// interface by simply wrapping it.
//
// Note that when caching things in a dependency context, you should
// be aware that the result values are stored in that context. The
// cache is only saving the call to the generator function. Once the value
// is in the dependency context, it will be there until that context is
// no longer in use. That is the lifetime rules of the cache do not apply
// to the values in the dependency context. The expectation is that the
// lifetime of the dependency context is far shorter than the lifetime
// of the cache.
//
// The TTL is the lifetime of the cache entry.
//
// If at least one of the result objects returned by the generator
// function implements the CacheTTL interface, the TTL for the cache
// entry will be the minimum of the TTL returned by the CacheTTL()
// method and the TTL parameter. If none of the result objects
// implement the CacheTTL interface, the TTL parameter will be used
// as the TTL for the cache entry.
func Cached(cache Cache, generator any, ttl time.Duration) any {
	opts := CtxCacheOptions{
		TTL: ttl,
	}
	return CachedOpts(cache, generator, opts)
}

// CachedCustom returns a function that caches the result of the given
// generator function. The cache key is generated by calling the
// CacheKey() method on the key parameter. The cache key must be
// unique for the given dependency and the given context. For parameters
// that do not implement the Keyable interface, the type name of the parameter
// is used as the cache key. This is to account for things that
// come from the dependency context that are purely structural. For example,
// a client connection library such as Resty.
//
// The intent is that you can easily adapt any cache to work with
// this function. For example, if you have a Redis cache, you can
// implement the Cache interface by calling the Redis commands.
// Or you can use a library like Ristretto to implement the Cache
// interface by simply wrapping it.
//
// Note that when caching things in a dependency context, you should
// be aware that the result values are stored in that context. The
// cache is only saving the call to the generator function. Once the value
// is in the dependency context, it will be there until that context is
// no longer in use. That is the lifetime rules of the cache do not apply
// to the values in the dependency context. The expectation is that the
// lifetime of the dependency context is far shorter than the lifetime
// of the cache.
//
// The TTL is the lifetime of the cache entry. The duration provider
// is a function that is called with the results of the generator function.
// The duration provider should return the TTL for the cache entry.
// The parameters passed to the duration provider is a slice of the non-error
// results of the generator function. The duration provider can use this
// to determine the TTL.
//
// If at least one of the result objects returned by the generator
// function implements the CacheTTL interface, the TTL for the cache
// entry will be the minimum of the TTL returned by the CacheTTL()
// method and what the duration provider returns. If none of the result
// objects implement the CacheTTL interface, the duration provider's result will
// be used as the TTL for the cache entry.
//
// This function is deprecated. Use CachedOpts instead.
func CachedCustom(cache Cache, generator any, durationProvider CacheDurationProvider) any {
	return CachedOpts(cache, generator, CtxCacheOptions{
		DurationProvider: durationProvider,
	})
}

// CachedOpts returns a function that caches the result of the given
// generator function. The cache key is generated by calling the
// CacheKey() method on the key parameter. The cache key must be
// unique for the given dependency and the given context. For parameters
// that do not implement the Keyable interface, the type name of the parameter
// is used as the cache key. This is to account for things that
// come from the dependency context that are purely structural. For example,
// a client connection library such as Resty.
//
// The intent is that you can easily adapt any cache to work with
// this function. For example, if you have a Redis cache, you can
// implement the Cache interface by calling the Redis commands.
// Or you can use a library like Ristretto to implement the Cache
// interface by simply wrapping it.
//
// Note that when caching things in a dependency context, you should
// be aware that the result values are stored in that context. The
// cache is only saving the call to the generator function. Once the value
// is in the dependency context, it will be there until that context is
// no longer in use. That is the lifetime rules of the cache do not apply
// to the values in the dependency context. The expectation is that the
// lifetime of the dependency context is far shorter than the lifetime
// of the cache.
//
// The opts parameter contains the options for the cache. Please see
// the CtxCacheOptions struct for more details on the options.
//
// If at least one of the result objects returned by the generator
// function implements the CacheTTL interface, the TTL for the cache
// entry will be the minimum of the TTL returned by the CacheTTL()
// method and the TTL parameter. If none of the result objects
// implement the CacheTTL interface, the TTL parameter will be used
// as the TTL for the cache entry.
func CachedOpts(cache Cache, generator any, opts CtxCacheOptions) any {
	if opts.DurationProvider == nil {
		opts.DurationProvider = DefaultDurationProvider
	}
	if opts.now == nil {
		opts.now = time.Now
	}

	genType := reflect.TypeOf(generator)
	if genType.Kind() != reflect.Func {
		panic("generator must be a function")
	}

	// Get this for later when we have to call it
	baseGenerator := reflect.ValueOf(generator)

	// Controls if we need to add a context parameter to the generator
	hasContext := false

	// Gather the input and output types
	inTypes := make([]reflect.Type, genType.NumIn())
	for i := 0; i < genType.NumIn(); i++ {
		in := genType.In(i)
		if in.ConvertibleTo(contextType) {
			hasContext = true
		}
		inTypes[i] = in
	}

	// If the generator does not take a context, add one. We'll remove
	// this later when we call the actual generator function. This causes
	// the returned function to have a different signature than the
	// generator function to allow for getting the appropriate context.
	// When calling the generator function, we'll remove the context
	// parameter.
	if !hasContext {
		inTypes = append(inTypes, contextType)
	}

	outTypes := make([]reflect.Type, genType.NumOut())
	for i := 0; i < genType.NumOut(); i++ {
		outTypes[i] = genType.Out(i)
	}

	returnTypeKey := generatorReturnTypes(outTypes)

	cachedGeneratorFunc := reflect.FuncOf(inTypes, outTypes, false)

	cacheLock := internalLock{}

	state := cacheState{
		opts:          opts,
		hasContext:    hasContext,
		baseGenerator: baseGenerator,
		cache:         cache,
		internalLock:  &cacheLock,
	}

	return reflect.MakeFunc(cachedGeneratorFunc, func(args []reflect.Value) []reflect.Value {
		var ctx context.Context
		for _, arg := range args {
			if arg.CanConvert(contextType) {
				ctx = arg.Interface().(context.Context)
				break
			}
		}
		cacheKey := generatorParamKeys(args) + "//" + returnTypeKey
		cachedValues := cache.Get(ctx, cacheKey)
		if cachedValues != nil {
			returnVals, savedTime, ttl := generateCacheResult(outTypes, cachedValues)
			handlePreRefresh(ctx, cacheKey, &state, savedTime, ttl)
			return returnVals
		}

		// If the cache supports locking, lock the key.
		unlock := cache.Lock(ctx, cacheKey)
		if unlock != nil {
			// If we have an unlocker, unlock the key when we return.
			defer unlock()
		}

		intUnlock, err := cacheLock.Lock(ctx, cacheKey)
		if intUnlock != nil {
			defer intUnlock()
		}
		if err != nil {
			// If we can't lock the key, just call the backing function
			// If this is due to a timeout, it's on the called function
			// to handle the timeout.
			return callBackingFunction(ctx, args, cacheKey, &state)
		}

		results := callBackingFunction(ctx, args, cacheKey, &state)

		return results
	}).Interface()
}

func handlePreRefresh(ctx context.Context, cacheKey string, state *cacheState, savedTime time.Time, ttl time.Duration) {
	opts := state.opts
	if opts.RefreshPercentage <= 0 || (opts.ForceRefreshPercentage <= opts.RefreshPercentage) {
		return
	}

	if !shouldPreRefresh(ttl, opts, state, savedTime) {
		return
	}

	prefetchKey := cacheKey + "-prefetch"
	isLocked, unlock := state.internalLock.LockOptional(prefetchKey)
	if unlock != nil {
		defer unlock()
	}
	if isLocked {
		// Someone else is already refreshing the cache entry.
		return
	}

	// Refresh the cache entry
	go func() {
		// At this point, we're inheriting the ctx of the caller. This is
		// so any timeouts associated with the caller are inherited by the
		// background goroutine. This is important because we don't want
		// the background goroutine to run forever.
		//
		// This is called at the same time a regular call to the backing
		// function would be called, so the expectation is that the backing
		// function is fast enough to not cause a timeout.
		defer func() {
			// Since we're on a background goroutine, we need to recover
			// from panics. We don't want to crash the program because of
			// a panic in a background goroutine.
			if p := recover(); p != nil {
				log.Printf("Panic in background goroutine refreshing cache: %v\n", p)
				buf := make([]byte, 1<<16)
				stackSize := runtime.Stack(buf, true)
				log.Printf("Stack trace: %s\n", buf[:stackSize])
			}
		}()

		// We don't need the return value since it's just a refresh, and we're not going to return anything
		_ = callBackingFunction(ctx, nil, cacheKey, state)
	}()
}

func shouldPreRefresh(ttl time.Duration, opts CtxCacheOptions, state *cacheState, savedTime time.Time) bool {
	slope, intercept := calculatePreRefreshCoefficients(ttl, opts)

	age := state.opts.now().Sub(savedTime).Seconds()
	percentage := slope*age + intercept

	if percentage < 0 {
		return false
	}

	if opts.RefreshAlpha <= 1 {
		return true
	}

	alpha := opts.RefreshAlpha
	probability := math.Pow(percentage, alpha-1)

	if probability <= 0 {
		return false
	}

	if rand.Float64() < probability {
		return false
	}

	return true
}

func calculatePreRefreshCoefficients(ttl time.Duration, opts CtxCacheOptions) (slope float64, intercept float64) {
	window := ttl.Seconds()
	forceRefreshPercentage := opts.ForceRefreshPercentage
	if forceRefreshPercentage <= 0 {
		forceRefreshPercentage = 1
	}
	effectiveWindow := window * (forceRefreshPercentage - opts.RefreshPercentage)
	scale := window / effectiveWindow
	slope = 1 / effectiveWindow
	intercept = 1 - scale
	return
}

type cacheState struct {
	opts          CtxCacheOptions
	hasContext    bool
	baseGenerator reflect.Value
	cache         Cache
	internalLock  *internalLock
}

func generateCacheResult(outTypes []reflect.Type, cachedValues []any) ([]reflect.Value, time.Time, time.Duration) {
	cachedValueIndex := 0 // Note that we don't cache errors, so the index can differ.
	returnVals := make([]reflect.Value, len(outTypes))
	for i, outType := range outTypes {
		if outType.ConvertibleTo(errorType) {
			// The cached results should not contain errors, so just make nil errors.
			returnVals[i] = reflect.Zero(outType)
		} else {
			// Populate the return value with the cached value.
			val := reflect.New(outType).Elem()
			cachedValue := reflect.ValueOf(cachedValues[cachedValueIndex])
			cachedValueIndex++
			val.Set(cachedValue)
			returnVals[i] = val
		}
	}

	saveTime := cachedValues[cachedValueIndex].(time.Time)
	ttl := cachedValues[cachedValueIndex+1].(time.Duration)

	return returnVals, saveTime, ttl
}

func callBackingFunction(ctx context.Context, args []reflect.Value, cacheKey string, state *cacheState) []reflect.Value {
	// If we added a context, then it'll be at the end. Remove it if we added it.
	funcArgs := args
	if !state.hasContext {
		funcArgs = funcArgs[:len(funcArgs)-1]
	}
	results := state.baseGenerator.Call(funcArgs)

	cacheVals := make([]any, 0)

	// Verify that the results are valid.
	for _, result := range results {
		if result.Type().ConvertibleTo(errorType) {
			if !result.IsNil() {
				// If there is an error, don't cache the result
				return results
			}
			continue
		} else if result.IsZero() {
			// If the result is nil, don't cache the result
			return results
		}

		cacheVals = append(cacheVals, result.Interface())
	}

	ttl := state.opts.DurationProvider(state.opts, cacheVals)
	now := state.opts.now()
	cacheVals = append(cacheVals, now)
	cacheVals = append(cacheVals, ttl)

	if ttl > 0 {
		state.cache.SetTTL(ctx, cacheKey, cacheVals, ttl)
	}
	return results
}

// generatorReturnTypes returns a string that represents the return
// types of the generator function. This is used to generate a unique
// signature for the given generator function.
func generatorReturnTypes(resultTypes []reflect.Type) string {
	builder := strings.Builder{}
	for _, resultType := range resultTypes {
		if resultType.ConvertibleTo(errorType) {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString(":")
		}
		builder.WriteString(resultType.Elem().Name())
	}
	return builder.String()
}

// generatorParamKeys returns a string that represents the parameters
// of the generator function.
func generatorParamKeys(args []reflect.Value) string {
	builder := strings.Builder{}
	for _, arg := range args {
		if arg.CanConvert(contextType) {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString(":")
		}
		val := arg.Interface()
		if keyable, ok := val.(Keyable); ok {
			builder.WriteString(keyable.CacheKey())
		} else if provider, ok := cacheKeyProviders[arg.Type()]; ok {
			builder.WriteString(provider(val))
		} else if stringer, ok := val.(fmt.Stringer); ok {
			builder.WriteString(stringer.String())
		} else {
			valJson, err := json.Marshal(val)
			if err != nil || string(valJson) == "{}" {
				builder.WriteString(arg.Type().Elem().Name())
			} else {
				builder.Write(valJson)
			}
		}
	}
	return builder.String()
}

// DefaultDurationProvider is a CacheDurationProvider that returns the
// minimum TTL of the given results that implement the CacheTTL interface.
// If none of the results implement the CacheTTL interface, the default
// TTL is returned.
func DefaultDurationProvider(opts CtxCacheOptions, rets []any) time.Duration {
	lowestTtl := opts.TTL
	for _, retObj := range rets {
		if cacheTTL, ok := retObj.(CacheTTL); ok {
			objectTTL := cacheTTL.CacheTTL()
			if objectTTL < lowestTtl {
				lowestTtl = objectTTL
			}
		}
	}
	return lowestTtl
}

type internalLock struct {
	mu   sync.Mutex
	keys map[string]*internalLockWait
}

func (il *internalLock) Lock(ctx context.Context, key string) (func(), error) {
	il.mu.Lock()
	if il.keys == nil {
		il.keys = make(map[string]*internalLockWait)
	}
	if _, ok := il.keys[key]; ok {
		// Already locked, add a wait.
		il.mu.Unlock()
		err := il.keys[key].wait(ctx)
		if err != nil {
			return nil, err
		}
		return il.Lock(ctx, key)
	}
	il.keys[key] = &internalLockWait{
		strobe: make([]chan struct{}, 0),
	}
	il.mu.Unlock()
	return func() {
		il.Unlock(key)
	}, nil
}

func (il *internalLock) LockOptional(key string) (bool, func()) {
	il.mu.Lock()
	defer il.mu.Unlock()
	if il.keys == nil {
		il.keys = make(map[string]*internalLockWait)
	}
	if _, ok := il.keys[key]; ok {
		// Already locked, return false that we did not get the lock.
		return false, nil
	}
	il.keys[key] = &internalLockWait{
		strobe: make([]chan struct{}, 0),
	}
	return true, func() {
		il.Unlock(key)
	}
}

func (il *internalLock) Unlock(key string) {
	il.mu.Lock()
	defer il.mu.Unlock()
	if _, ok := il.keys[key]; ok {
		il.keys[key].release()
	}
	delete(il.keys, key)
}

type internalLockWait struct {
	mu     sync.Mutex
	strobe []chan struct{}
}

func (ilw *internalLockWait) wait(ctx context.Context) error {
	ch := make(chan struct{})
	ilw.mu.Lock()
	ilw.strobe = append(ilw.strobe, ch)
	ilw.mu.Unlock()
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (ilw *internalLockWait) release() {
	ilw.mu.Lock()
	defer ilw.mu.Unlock()
	for _, ch := range ilw.strobe {
		close(ch)
	}
	ilw.strobe = nil
}
