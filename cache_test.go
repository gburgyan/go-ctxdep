package ctxdep

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
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
}

func (d *DumbCache) Get(ctx context.Context, key string) []any {
	value, ok := d.values[key]
	if !ok {
		return nil
	}
	return value
}

func (d *DumbCache) SetTTL(ctx context.Context, key string, value []any, ttl time.Duration) {
	d.values[key] = value
}

func (d *DumbCache) Lock(ctx context.Context, key string) func() {
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

func Test_CacheComplex(t *testing.T) {
	cache := DumbCache{
		values: make(map[string][]any),
	}

	callCount := 0
	generator := func(key1 *inputValue, key2 *inputValue2) (*outputValue, *testDoodad, error) {
		callCount++
		return &outputValue{Value: key1.Value}, &testDoodad{val: key2.Value}, nil
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
	assert.Equal(t, "2", r1b.val)
	assert.Equal(t, "1", r2a.Value)
	assert.Equal(t, "2", r2b.val)
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

func Test_Cache_NonCacheable(t *testing.T) {
	cache := DumbCache{
		values: make(map[string][]any),
	}

	generator := func(ctx context.Context, widget *testWidget) (*outputValue, error) {
		return nil, nil
	}

	assert.PanicsWithValue(t, "generator must take a parameters of context or Keyable", func() {
		NewDependencyContext(context.Background(), &testWidget{}, Cached(&cache, generator, time.Minute))
	})
}
