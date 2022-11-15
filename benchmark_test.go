package ctxdep

import (
	"context"
	"testing"
)

func BenchmarkGetStruct(b *testing.B) {
	ctx := NewDependencyContext(context.Background(), &testWidget{42})

	for i := 0; i < b.N; i++ {
		_ = Get[*testWidget](ctx)
	}
}

func BenchmarkGetInterface(b *testing.B) {
	ctx := NewDependencyContext(context.Background(), func(ctx context.Context) *tempImpl {
		return &tempImpl{}
	})

	for i := 0; i < b.N; i++ {
		_ = Get[tempInterface](ctx)
	}
}