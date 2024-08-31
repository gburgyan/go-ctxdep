package ctxdep

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"math"
	"reflect"
	"strconv"
	"testing"
	"time"
)

type inputValue struct {
	Value string
}

func (i *inputValue) CacheKey() string {
	return i.Value
}

type inputValue2 struct {
	Value string
}

func (i *inputValue2) CacheKey() string {
	return i.Value
}

type outputValue struct {
	Value string
}

type DumbCache struct {
	values      map[string][]any
	lockCount   int
	unlockCount int
	lastTtl     time.Duration
}

func (d *DumbCache) Get(ctx context.Context, key string) []any {
	if ctx == nil {
		panic("ctx is nil")
	}
	value, ok := d.values[key]
	if !ok {
		return nil
	}
	return value
}

func (d *DumbCache) SetTTL(ctx context.Context, key string, value []any, ttl time.Duration) {
	if ctx == nil {
		panic("ctx is nil")
	}
	d.values[key] = value
	d.lastTtl = ttl
}

func (d *DumbCache) Lock(ctx context.Context, key string) func() {
	if ctx == nil {
		panic("ctx is nil")
	}
	d.lockCount++
	return func() {
		d.unlockCount++
	}
}

func Test_Cache(t *testing.T) {
	cache := DumbCache{
		values: make(map[string][]any),
	}

	callCount := 0
	generator := func(ctx context.Context, key *inputValue) (*outputValue, error) {
		callCount++
		return &outputValue{Value: key.Value}, nil
	}

	input := &inputValue{Value: "1"}

	ctx1 := NewDependencyContext(context.Background(), input, Cached(&cache, generator, time.Minute))
	r1 := Get[*outputValue](ctx1)

	ctx2 := NewDependencyContext(context.Background(), input, Cached(&cache, generator, time.Minute))
	r2 := Get[*outputValue](ctx2)

	assert.Contains(t, cache.values, "1//outputValue")
	assert.Equal(t, 1, callCount)
	assert.Equal(t, "1", r1.Value)
	assert.Equal(t, "1", r2.Value)
	assert.Equal(t, 1, cache.lockCount)
	assert.Equal(t, 1, cache.unlockCount)
}

func Test_CacheCustom(t *testing.T) {
	cache := DumbCache{
		values: make(map[string][]any),
	}

	callCount := 0
	generator := func(ctx context.Context, key *inputValue) (*outputValue, error) {
		callCount++
		return &outputValue{Value: key.Value}, nil
	}

	input := &inputValue{Value: "42"}
	var retDuration time.Duration

	ttlProvider := func(opts CtxCacheOptions, anies []any) time.Duration {
		outVal := anies[0].(*outputValue)
		// Convert the output value to an int, and use that as the TTL in minutes.
		i, err := strconv.Atoi(outVal.Value)
		if err != nil {
			return time.Minute
		}
		retDuration = time.Duration(i) * time.Minute
		return retDuration
	}

	ctx1 := NewDependencyContext(context.Background(), input, CachedCustom(&cache, generator, ttlProvider))
	r1 := Get[*outputValue](ctx1)

	assert.Contains(t, cache.values, "42//outputValue")
	assert.Equal(t, 1, callCount)
	assert.Equal(t, "42", r1.Value)

	assert.Equal(t, time.Minute*42, retDuration)
}

func Test_CacheComplex(t *testing.T) {
	cache := DumbCache{
		values: make(map[string][]any),
	}

	callCount := 0
	generator := func(key1 *inputValue, key2 *inputValue2) (*outputValue, *testDoodad, error) {
		callCount++
		return &outputValue{Value: key1.Value}, &testDoodad{Val: key2.Value}, nil
	}

	input := &inputValue{Value: "1"}
	input2 := &inputValue2{Value: "2"}

	ctx1 := NewDependencyContext(context.Background(), input, input2, Cached(&cache, generator, time.Minute))
	r1a := Get[*outputValue](ctx1)
	r1b := Get[*testDoodad](ctx1)

	ctx2 := NewDependencyContext(context.Background(), input, input2, Cached(&cache, generator, time.Minute))
	r2a := Get[*outputValue](ctx2)
	r2b := Get[*testDoodad](ctx2)

	assert.Contains(t, cache.values, "1:2//outputValue:testDoodad")
	assert.Equal(t, 1, callCount)
	assert.Equal(t, "1", r1a.Value)
	assert.Equal(t, "2", r1b.Val)
	assert.Equal(t, "1", r2a.Value)
	assert.Equal(t, "2", r2b.Val)
	assert.Equal(t, 1, cache.lockCount)
	assert.Equal(t, 1, cache.unlockCount)
}

func Test_Cache_Error(t *testing.T) {
	cache := DumbCache{
		values: make(map[string][]any),
	}

	callCount := 0
	generator := func(ctx context.Context, key *inputValue) (*outputValue, error) {
		callCount++
		return &outputValue{}, fmt.Errorf("error")
	}

	input := &inputValue{Value: "1"}

	ctx := NewDependencyContext(context.Background(), input, Cached(&cache, generator, time.Minute))

	_, err := GetWithError[*outputValue](ctx)
	assert.Error(t, err)
	_, err = GetWithError[*outputValue](ctx)
	assert.Error(t, err)

	assert.Len(t, cache.values, 0)
	assert.Equal(t, 2, callCount)
}

func Test_Cache_Nil(t *testing.T) {
	cache := DumbCache{
		values: make(map[string][]any),
	}

	callCount := 0
	generator := func(ctx context.Context, key *inputValue) (*outputValue, error) {
		callCount++
		// Not an error, but also nil, which shouldn't be cached as it's not
		// valid for a generator to return.
		return nil, nil
	}

	input := &inputValue{Value: "1"}

	ctx := NewDependencyContext(context.Background(), input, Cached(&cache, generator, time.Minute))

	_, err := GetWithError[*outputValue](ctx)
	assert.Error(t, err)
	_, err = GetWithError[*outputValue](ctx)
	assert.EqualError(t, err, "error mapping generator results to context: *ctxdep.outputValue (generator returned nil result: *ctxdep.outputValue)")

	assert.Len(t, cache.values, 0)
	assert.Equal(t, 2, callCount)
}

func Test_Cache_NonFunction(t *testing.T) {
	cache := DumbCache{
		values: make(map[string][]any),
	}

	input := &inputValue{Value: "1"}

	assert.PanicsWithValue(t, "generator must be a function", func() {
		NewDependencyContext(context.Background(), input, Cached(&cache, "NotFunction", time.Minute))
	})
}

func Test_Cache_NonKeyed_JSON(t *testing.T) {
	cache := DumbCache{
		values: make(map[string][]any),
	}

	generator := func(ctx context.Context, widget *testWidget) (*outputValue, error) {
		return &outputValue{Value: strconv.Itoa(widget.Val)}, nil
	}

	ctx := NewDependencyContext(context.Background(), &testWidget{Val: 42}, Cached(&cache, generator, time.Minute))
	ov := Get[*outputValue](ctx)
	assert.Contains(t, cache.values, "{\"Val\":42}//outputValue")
	assert.Equal(t, "42", ov.Value)
}

func Test_Cache_NonKeyed_EmptyJSON(t *testing.T) {
	cache := DumbCache{
		values: make(map[string][]any),
	}

	type empty struct {
	}

	generator := func(ctx context.Context, e *empty) (*testWidget, error) {
		return &testWidget{Val: 42}, nil
	}

	ctx := NewDependencyContext(context.Background(), &empty{}, Cached(&cache, generator, time.Minute))
	ov := Get[*testWidget](ctx)
	assert.Contains(t, cache.values, "empty//testWidget")
	assert.NotNil(t, ov)
}

func Test_Cache_NonKeyed_Stringer(t *testing.T) {
	cache := DumbCache{
		values: make(map[string][]any),
	}

	generator := func(ctx context.Context, doodad *testDoodad) (*outputValue, error) {
		return &outputValue{Value: doodad.Val}, nil
	}

	ctx := NewDependencyContext(context.Background(), &testDoodad{Val: "42"}, Cached(&cache, generator, time.Minute))
	ov := Get[*outputValue](ctx)
	assert.Contains(t, cache.values, "42//outputValue")
	assert.Equal(t, "42", ov.Value)
}

func Test_Cache_NonKeyed_BadJSON(t *testing.T) {
	cache := DumbCache{
		values: make(map[string][]any),
	}

	type recursive struct {
		Val  string
		Next *recursive
	}

	generator := func(ctx context.Context, recursive *recursive) (*outputValue, error) {
		return &outputValue{Value: recursive.Val}, nil
	}

	input := &recursive{
		Val: "42",
	}
	input.Next = input // Purposefully create a recursive structure that can't be marshalled to JSON.

	ctx := NewDependencyContext(context.Background(), input, Cached(&cache, generator, time.Minute))
	ov := Get[*outputValue](ctx)
	assert.Contains(t, cache.values, "recursive//outputValue")
	assert.Equal(t, "42", ov.Value)
}

func Test_Cache_NonKeyed_CustomKey(t *testing.T) {
	cache := DumbCache{
		values: make(map[string][]any),
	}

	generator := func(ctx context.Context, widget *testWidget) (*outputValue, error) {
		return &outputValue{Value: strconv.Itoa(widget.Val)}, nil
	}

	RegisterCacheKeyProvider(reflect.TypeOf(&testWidget{}), func(any any) string {
		widget := any.(*testWidget)
		return fmt.Sprintf("custom:%d", widget.Val)
	})

	ctx := NewDependencyContext(context.Background(), &testWidget{Val: 42}, Cached(&cache, generator, time.Minute))
	ov := Get[*outputValue](ctx)
	assert.Contains(t, cache.values, "custom:42//outputValue")
	assert.Equal(t, "42", ov.Value)
}

type testCacheTTL struct {
	minutes int
}

func (t *testCacheTTL) CacheTTL() time.Duration {
	return time.Duration(t.minutes) * time.Minute
}

func Test_Cache_ObjectTTL(t *testing.T) {
	cache := DumbCache{
		values: make(map[string][]any),
	}

	generator := func(ctx context.Context, key *inputValue) (*testCacheTTL, error) {
		i, err := strconv.Atoi(key.Value)
		if err != nil {
			return nil, err
		}

		return &testCacheTTL{minutes: i}, nil
	}

	input := &inputValue{Value: "42"}

	ctx1 := NewDependencyContext(context.Background(), input, Cached(&cache, generator, time.Hour))
	r1 := Get[*testCacheTTL](ctx1)

	assert.Contains(t, cache.values, "42//testCacheTTL")
	assert.Equal(t, 42, r1.minutes)

	assert.Equal(t, time.Minute*42, cache.lastTtl)
}

func Test_Cache_ObjectTTL_Zero(t *testing.T) {
	cache := DumbCache{
		values: make(map[string][]any),
	}

	generator := func(ctx context.Context, key *inputValue) (*testCacheTTL, error) {
		i, err := strconv.Atoi(key.Value)
		if err != nil {
			return nil, err
		}

		return &testCacheTTL{minutes: i}, nil
	}

	input := &inputValue{Value: "0"}

	ctx1 := NewDependencyContext(context.Background(), input, Cached(&cache, generator, time.Hour))
	r1 := Get[*testCacheTTL](ctx1)

	assert.Empty(t, cache.values)
	assert.Equal(t, 0, r1.minutes)
}

func Test_Lock_AlreadyLocked(t *testing.T) {
	il := &internalLock{}
	ctx := context.Background()
	key := "testKey"

	// First lock
	unlock1, err := il.Lock(ctx, key)
	assert.NoError(t, err)
	assert.NotNil(t, unlock1)

	// Second lock should wait
	done := make(chan struct{})
	go func() {
		defer close(done)
		unlock2, err := il.Lock(ctx, key)
		assert.NoError(t, err)
		assert.NotNil(t, unlock2)
		unlock2()
	}()

	select {
	case <-done:
		t.Fatal("Second lock should not have succeeded immediately")
	case <-time.After(100 * time.Millisecond):
		// Expected to timeout
	}

	// Unlock the first lock
	unlock1()

	// Now the second lock should succeed
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Second lock should have succeeded after first unlock")
	}
}

func Test_Lock_ContextCancelled(t *testing.T) {
	il := &internalLock{}
	ctx, cancel := context.WithCancel(context.Background())
	key := "testKey"

	// First lock
	unlock1, err := il.Lock(ctx, key)
	assert.NoError(t, err)
	assert.NotNil(t, unlock1)

	// Cancel the context
	cancel()

	// Second lock should fail due to cancelled context
	_, err = il.Lock(ctx, key)
	assert.Error(t, err)

	// Unlock the first lock
	unlock1()
}

func Test_Lock_NoKeys(t *testing.T) {
	il := &internalLock{}
	ctx := context.Background()
	key := "testKey"

	// Lock with no keys initialized
	unlock, err := il.Lock(ctx, key)
	assert.NoError(t, err)
	assert.NotNil(t, unlock)

	// Ensure the key is locked
	_, ok := il.keys[key]
	assert.True(t, ok)

	// Unlock the key
	unlock()
	_, ok = il.keys[key]
	assert.False(t, ok)
}

func Test_LockOptional_Success(t *testing.T) {
	il := &internalLock{}
	key := "testKey"

	locked, unlock := il.LockOptional(key)
	assert.True(t, locked)
	assert.NotNil(t, unlock)

	_, ok := il.keys[key]
	assert.True(t, ok)

	unlock()
	_, ok = il.keys[key]
	assert.False(t, ok)
}

func Test_LockOptional_AlreadyLocked(t *testing.T) {
	il := &internalLock{}
	key := "testKey"

	locked1, unlock1 := il.LockOptional(key)
	assert.True(t, locked1)
	assert.NotNil(t, unlock1)

	locked2, unlock2 := il.LockOptional(key)
	assert.False(t, locked2)
	assert.Nil(t, unlock2)

	unlock1()
	_, ok := il.keys[key]
	assert.False(t, ok)
}

func Test_LockOptional_NoKeysInitialized(t *testing.T) {
	il := &internalLock{}
	key := "testKey"

	locked, unlock := il.LockOptional(key)
	assert.True(t, locked)
	assert.NotNil(t, unlock)

	_, ok := il.keys[key]
	assert.True(t, ok)

	unlock()
	_, ok = il.keys[key]
	assert.False(t, ok)
}

func Test_CalculatePreRefreshCoefficients_ValidInputs(t *testing.T) {
	opts := CtxCacheOptions{
		RefreshPercentage:      0.5,
		ForceRefreshPercentage: 0.8,
	}
	ttl := time.Minute * 10

	slope, intercept := calculatePreRefreshCoefficients(ttl, opts)

	assert.InDelta(t, 0.00555555, slope, 0.0001)
	assert.InDelta(t, -1.66666666, intercept, 0.0001)
}

func Test_CalculatePreRefreshCoefficients_EqualPercentages(t *testing.T) {
	opts := CtxCacheOptions{
		RefreshPercentage:      0.5,
		ForceRefreshPercentage: 0.5,
	}
	ttl := time.Minute * 10

	slope, intercept := calculatePreRefreshCoefficients(ttl, opts)

	assert.True(t, math.IsInf(slope, 1))
	assert.True(t, math.IsInf(intercept, -1))
}

func Test_CalculatePreRefreshCoefficients_NoForceRefresh(t *testing.T) {
	opts := CtxCacheOptions{
		RefreshPercentage:      0.5,
		ForceRefreshPercentage: 0,
	}
	ttl := time.Minute * 10

	slope, intercept := calculatePreRefreshCoefficients(ttl, opts)

	assert.InDelta(t, 0.0033333, slope, 0.0001)
	assert.InDelta(t, -1, intercept, 0.0001)
}

func Test_shouldPreRefresh_StartOfTTL(t *testing.T) {
	now := time.Now()
	opts := CtxCacheOptions{
		RefreshPercentage:      0.5,
		ForceRefreshPercentage: 0.8,
		RefreshAlpha:           2,
		now:                    func() time.Time { return now },
	}
	state := &cacheState{opts: opts}
	savedTime := now
	ttl := time.Minute * 10

	result := shouldPreRefresh(ttl, opts, state, savedTime)

	assert.False(t, result)
}

func Test_shouldPreRefresh_EndOfTTL(t *testing.T) {
	now := time.Now()
	opts := CtxCacheOptions{
		RefreshPercentage:      0.5,
		ForceRefreshPercentage: 0.8,
		RefreshAlpha:           2,
		now:                    func() time.Time { return now },
	}
	state := &cacheState{opts: opts}
	savedTime := now.Add(-time.Minute * 10)
	ttl := time.Minute * 10

	result := shouldPreRefresh(ttl, opts, state, savedTime)

	assert.True(t, result)
}

func Test_shouldPreRefresh_AlphaLessThanOrEqualToOne(t *testing.T) {
	now := time.Now()
	opts := CtxCacheOptions{
		RefreshPercentage:      0.5,
		ForceRefreshPercentage: 0.8,
		RefreshAlpha:           1,
		now:                    func() time.Time { return now },
	}
	state := &cacheState{opts: opts}
	savedTime := now.Add(-time.Minute * 5)
	ttl := time.Minute * 10

	result := shouldPreRefresh(ttl, opts, state, savedTime)

	assert.True(t, result)
}

func Test_shouldPreRefresh_ProbOne(t *testing.T) {
	now := time.Now()
	opts := CtxCacheOptions{
		RefreshPercentage:      0.5,
		ForceRefreshPercentage: 0.8,
		RefreshAlpha:           1.000000000001,
		now:                    func() time.Time { return now },
	}
	state := &cacheState{opts: opts}
	savedTime := now.Add(-time.Minute*8 - time.Microsecond)
	ttl := time.Minute * 10

	result := shouldPreRefresh(ttl, opts, state, savedTime)

	assert.True(t, result)
}

func Test_shouldPreRefresh_ProbZero(t *testing.T) {
	now := time.Now()
	opts := CtxCacheOptions{
		RefreshPercentage:      0.5,
		ForceRefreshPercentage: 0.8,
		RefreshAlpha:           1000,
		now:                    func() time.Time { return now },
	}
	state := &cacheState{opts: opts}
	savedTime := now.Add(-time.Minute*5 - time.Microsecond)
	ttl := time.Minute * 10

	result := shouldPreRefresh(ttl, opts, state, savedTime)

	assert.False(t, result)
}
