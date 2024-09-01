package ctxdep

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
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

	RegisterCacheKeyProvider(reflect.TypeOf(&testWidget{}), func(any any) ([]byte, error) {
		widget := any.(*testWidget)
		return []byte(fmt.Sprintf("custom:%d", widget.Val)), nil
	})

	ctx := NewDependencyContext(context.Background(), &testWidget{Val: 42}, Cached(&cache, generator, time.Minute))
	ov := Get[*outputValue](ctx)
	assert.Contains(t, cache.values, "custom:42//outputValue")
	assert.Equal(t, "42", ov.Value)
}

func Test_Cache_Interface(t *testing.T) {
	cache := DumbCache{
		values: make(map[string][]any),
	}

	generator := func(ctx context.Context, ti testInterface) (*outputValue, error) {
		return &outputValue{Value: strconv.Itoa(ti.getVal())}, nil
	}

	RegisterCacheKeyProvider(reflect.TypeOf((*testInterface)(nil)).Elem(), func(any any) ([]byte, error) {
		iface := any.(testInterface)
		key := fmt.Sprintf("interface:%d", iface.getVal())
		return []byte(key), nil
	})

	ctx := NewDependencyContext(context.Background(), &testImpl{val: 42}, Cached(&cache, generator, time.Minute))
	ov := Get[*outputValue](ctx)
	assert.Contains(t, cache.values, "interface:42//outputValue")
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
	unlock1, err := il.lock(ctx, key)
	assert.NoError(t, err)
	assert.NotNil(t, unlock1)

	// Second lock should wait
	done := make(chan struct{})
	go func() {
		defer close(done)
		unlock2, err := il.lock(ctx, key)
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

	// unlock the first lock
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
	unlock1, err := il.lock(ctx, key)
	assert.NoError(t, err)
	assert.NotNil(t, unlock1)

	// Cancel the context
	cancel()

	// Second lock should fail due to cancelled context
	_, err = il.lock(ctx, key)
	assert.Error(t, err)

	// Unlock the first lock
	unlock1()
}

func Test_Lock_NoKeys(t *testing.T) {
	il := &internalLock{}
	ctx := context.Background()
	key := "testKey"

	// lock with no keys initialized
	unlock, err := il.lock(ctx, key)
	assert.NoError(t, err)
	assert.NotNil(t, unlock)

	// Ensure the key is locked
	_, ok := il.keys[key]
	assert.True(t, ok)

	// unlock the key
	unlock()
	_, ok = il.keys[key]
	assert.False(t, ok)
}

func Test_LockOptional_Success(t *testing.T) {
	il := &internalLock{}
	key := "testKey"

	locked, unlock := il.lockOptional(key)
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

	locked1, unlock1 := il.lockOptional(key)
	assert.True(t, locked1)
	assert.NotNil(t, unlock1)

	locked2, unlock2 := il.lockOptional(key)
	assert.False(t, locked2)
	assert.Nil(t, unlock2)

	unlock1()
	_, ok := il.keys[key]
	assert.False(t, ok)
}

func Test_LockOptional_NoKeysInitialized(t *testing.T) {
	il := &internalLock{}
	key := "testKey"

	locked, unlock := il.lockOptional(key)
	assert.True(t, locked)
	assert.NotNil(t, unlock)

	_, ok := il.keys[key]
	assert.True(t, ok)

	unlock()
	_, ok = il.keys[key]
	assert.False(t, ok)
}

func Test_shouldPreRefresh_Yes(t *testing.T) {
	now := time.Now()
	opts := CtxCacheOptions{
		RefreshPercentage: 0.5,
		now:               func() time.Time { return now },
	}
	state := &cacheState{opts: opts}
	savedTime := now.Add(-time.Minute * 5)
	ttl := time.Minute * 10

	result := shouldPreRefresh(state, ttl, savedTime)

	assert.True(t, result)
}

func Test_handlePreRefresh_AlreadyLocked(t *testing.T) {
	ctx := context.Background()
	cacheKey := "testKey"
	now := time.Now()
	cache := DumbCache{
		values: make(map[string][]any),
	}
	calls := 0
	f := func(s string) *string {
		calls++
		return &s
	}
	options := CtxCacheOptions{
		RefreshPercentage: 0.5,
		now:               func() time.Time { return now },
	}
	state := makeStateForGenerator(&cache, f, options)

	savedTime := now.Add(-time.Minute * 6)
	ttl := time.Minute * 10

	unlock, err := state.internalLock.lock(ctx, cacheKey+"-prefetch")
	assert.NoError(t, err)
	defer unlock()

	args := []reflect.Value{
		reflect.ValueOf("test"),
	}

	handlePreRefresh(ctx, cacheKey, state, args, savedTime, ttl)
	time.Sleep(time.Millisecond * 10)

	assert.Equal(t, 0, calls)
}

func Test_handlePreRefresh_HappyCase(t *testing.T) {
	ctx := context.Background()
	cacheKey := "testKey"
	now := time.Now()
	cache := DumbCache{
		values: make(map[string][]any),
	}
	calls := 0
	f := func(s string) *string {
		calls++
		return &s
	}
	options := CtxCacheOptions{
		RefreshPercentage: 0.5,
		DurationProvider:  DefaultDurationProvider,
		now:               func() time.Time { return now },
	}
	state := makeStateForGenerator(&cache, f, options)

	savedTime := now.Add(-time.Minute * 6)
	ttl := time.Minute * 10

	args := []reflect.Value{
		reflect.ValueOf("test"),
		reflect.ValueOf(ctx),
	}

	handlePreRefresh(ctx, cacheKey, state, args, savedTime, ttl)
	time.Sleep(time.Millisecond * 10)
	assert.Equal(t, 1, calls)
}

func Test_handlePreRefresh_TooNew(t *testing.T) {
	ctx := context.Background()
	cacheKey := "testKey"
	now := time.Now()
	cache := DumbCache{
		values: make(map[string][]any),
	}
	calls := 0
	f := func(s string) *string {
		calls++
		return &s
	}
	options := CtxCacheOptions{
		RefreshPercentage: 0.5,
		DurationProvider:  DefaultDurationProvider,
		now:               func() time.Time { return now },
	}
	state := makeStateForGenerator(&cache, f, options)

	savedTime := now.Add(-time.Minute * 2)
	ttl := time.Minute * 10

	args := []reflect.Value{
		reflect.ValueOf("test"),
		reflect.ValueOf(ctx),
	}

	handlePreRefresh(ctx, cacheKey, state, args, savedTime, ttl)
	time.Sleep(time.Millisecond * 10)
	assert.Equal(t, 0, calls)
}

func Test_handlePreRefresh_Panics(t *testing.T) {
	ctx := context.Background()
	cacheKey := "testKey"
	now := time.Now()
	cache := DumbCache{
		values: make(map[string][]any),
	}
	calls := 0
	f := func(s string) *string {
		calls++
		panic("test panic")
	}
	options := CtxCacheOptions{
		RefreshPercentage: 0.5,
		DurationProvider:  DefaultDurationProvider,
		now:               func() time.Time { return now },
	}
	state := makeStateForGenerator(&cache, f, options)

	savedTime := now.Add(-time.Minute * 9)
	ttl := time.Minute * 10

	args := []reflect.Value{
		reflect.ValueOf("test"),
		reflect.ValueOf(ctx),
	}

	// The function panics, but the panic is caught and the function continues.
	handlePreRefresh(ctx, cacheKey, state, args, savedTime, ttl)
	time.Sleep(time.Millisecond * 10)
	assert.Equal(t, 1, calls)
}
