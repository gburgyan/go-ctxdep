package ctxdep

import (
	"context"
	"testing"
	"time"
)

// Benchmark specifically for interface lookup performance
func BenchmarkInterfaceLookup(b *testing.B) {
	// Create a context with many concrete types
	ctx := NewDependencyContext(context.Background(),
		&testImpl{val: 1},
		&testWidget{Val: 2},
		&testDoodad{Val: "3"},
	)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// This will trigger the interface lookup
		var iface testInterface
		GetBatch(ctx, &iface)
	}
}

// Benchmark for repeated interface lookups (should benefit from cache)
func BenchmarkRepeatedInterfaceLookup(b *testing.B) {
	// Create many contexts, each will benefit from the global type cache
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx := NewDependencyContext(context.Background(),
			&testImpl{val: i},
		)
		var iface testInterface
		GetBatch(ctx, &iface)
	}
}

// testKeyable implements Keyable for cache benchmarks
type testKeyable struct {
	id string
}

func (t *testKeyable) CacheKey() string {
	return t.id
}

// Simple cache for benchmarking
type benchCache struct {
	data map[string][]any
}

func (b *benchCache) Get(ctx context.Context, key string) []any {
	return b.data[key]
}

func (b *benchCache) SetTTL(ctx context.Context, key string, value []any, ttl time.Duration) {
	b.data[key] = value
}

// Benchmark cache key generation with Keyable interface
func BenchmarkCacheKeyGeneration(b *testing.B) {
	cache := &benchCache{data: make(map[string][]any)}
	gen := Cached(
		cache,
		func(ctx context.Context, k *testKeyable) (*testWidget, error) {
			return &testWidget{Val: len(k.id)}, nil
		},
		time.Hour,
	)

	key := &testKeyable{id: "test-key-123"}
	ctx := NewDependencyContext(context.Background(), gen, key)

	// Pre-warm the cache
	var widget *testWidget
	GetBatch(ctx, &widget)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GetBatch(ctx, &widget)
	}
}
