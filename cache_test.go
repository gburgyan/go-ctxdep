package ctxdep

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

type InputValue struct {
	Value string
}

func (i *InputValue) CacheKey() string {
	return i.Value
}

type OutputValue struct {
	Value string
}

type DumbCache struct {
	values map[string]any
}

func (d *DumbCache) Get(key string) any {
	value, ok := d.values[key]
	if !ok {
		return nil
	}
	return value
}

func (d *DumbCache) SetTTL(key string, value any, ttl time.Duration) {
	d.values[key] = value
}

func Test_Cacheable(t *testing.T) {
	cache := DumbCache{
		values: make(map[string]any),
	}

	callCount := 0
	generator := func(ctx context.Context, key *InputValue) (*OutputValue, error) {
		callCount++
		return &OutputValue{Value: key.Value}, nil
	}

	input := &InputValue{Value: "1"}

	ctx := NewDependencyContext(context.Background(), input, Cacheable(&cache, generator, time.Minute))

	r1 := Get[*OutputValue](ctx)
	r2 := Get[*OutputValue](ctx)

	assert.Contains(t, cache.values, "DepCache:(1)->OutputValue")
	assert.Equal(t, 1, callCount)
	assert.Equal(t, "1", r1.Value)
	assert.Equal(t, "1", r2.Value)
}

func Test_Cacheable_Error(t *testing.T) {
	cache := DumbCache{
		values: make(map[string]any),
	}

	callCount := 0
	generator := func(ctx context.Context, key *InputValue) (*OutputValue, error) {
		callCount++
		return nil, fmt.Errorf("error")
	}

	input := &InputValue{Value: "1"}

	ctx := NewDependencyContext(context.Background(), input, Cacheable(&cache, generator, time.Minute))

	_, err := GetWithError[*OutputValue](ctx)
	assert.Error(t, err)
	_, err = GetWithError[*OutputValue](ctx)
	assert.Error(t, err)

	assert.Len(t, cache.values, 0)
	assert.Equal(t, 2, callCount)
}
