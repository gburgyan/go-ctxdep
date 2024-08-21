package ctxdep

import (
	"context"
	"reflect"
	"testing"
)

func BenchmarkGetStruct(b *testing.B) {
	ctx := NewDependencyContext(context.Background(), &testWidget{42})

	for i := 0; i < b.N; i++ {
		_ = Get[*testWidget](ctx)
	}
}

func BenchmarkGetInterface(b *testing.B) {
	ctx := NewDependencyContext(context.Background(), func(ctx context.Context) *testImpl {
		return &testImpl{}
	})

	for i := 0; i < b.N; i++ {
		_ = Get[testInterface](ctx)
	}
}

func BenchmarkGetSimpleGenerator(b *testing.B) {
	ctx := NewDependencyContext(context.Background(), func() *testWidget {
		return &testWidget{Val: 42}
	})
	dc := GetDependencyContext(ctx)
	t := reflect.TypeOf(&testWidget{})
	sa, _ := dc.slots.Load(t)
	s := sa.(*slot)

	for i := 0; i < b.N; i++ {
		_ = Get[*testWidget](ctx)
		// Intentionally clear the generated value using a non-public value.
		s.value = nil
	}
}

func BenchmarkGetGeneratorWithDependency(b *testing.B) {
	ctx := NewDependencyContext(context.Background(), func() *testWidget {
		return &testWidget{Val: 42}
	}, func(_ *testWidget) *testDoodad {
		return &testDoodad{Val: "105"}
	})
	dc := GetDependencyContext(ctx)
	t := reflect.TypeOf(&testDoodad{})
	sa, _ := dc.slots.Load(t)
	s := sa.(*slot)

	for i := 0; i < b.N; i++ {
		_ = Get[*testDoodad](ctx)
		// Intentionally clear the generated value using a non-public value.
		s.value = nil
	}
}

func BenchmarkGetGeneratorWithDependencyAndContext(b *testing.B) {
	ctx := NewDependencyContext(context.Background(), func() *testWidget {
		return &testWidget{Val: 42}
	}, func(_ context.Context, _ *testWidget) *testDoodad {
		return &testDoodad{Val: "105"}
	})
	dc := GetDependencyContext(ctx)
	t := reflect.TypeOf(&testDoodad{})
	sa, _ := dc.slots.Load(t)
	s := sa.(*slot)

	for i := 0; i < b.N; i++ {
		_ = Get[*testDoodad](ctx)
		// Intentionally clear the generated value using a non-public value.
		s.value = nil
	}
}
