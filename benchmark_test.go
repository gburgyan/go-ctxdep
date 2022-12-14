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
		return &testWidget{val: 42}
	})
	dc := GetDependencyContext(ctx)
	t := reflect.TypeOf(&testWidget{})
	s := dc.slots[t]

	for i := 0; i < b.N; i++ {
		_ = Get[*testWidget](ctx)
		// Intentionally clear the generated value using a non-public value.
		s.value = nil
	}
}

func BenchmarkGetGeneratorWithDepenency(b *testing.B) {
	ctx := NewDependencyContext(context.Background(), func() *testWidget {
		return &testWidget{val: 42}
	}, func(_ *testWidget) *testDoodad {
		return &testDoodad{val: "105"}
	})
	dc := GetDependencyContext(ctx)
	t := reflect.TypeOf(&testDoodad{})
	s := dc.slots[t]

	for i := 0; i < b.N; i++ {
		_ = Get[*testDoodad](ctx)
		// Intentionally clear the generated value using a non-public value.
		s.value = nil
	}
}

func BenchmarkGetGeneratorWithDepenencyAndContext(b *testing.B) {
	ctx := NewDependencyContext(context.Background(), func() *testWidget {
		return &testWidget{val: 42}
	}, func(_ context.Context, _ *testWidget) *testDoodad {
		return &testDoodad{val: "105"}
	})
	dc := GetDependencyContext(ctx)
	t := reflect.TypeOf(&testDoodad{})
	s := dc.slots[t]

	for i := 0; i < b.N; i++ {
		_ = Get[*testDoodad](ctx)
		// Intentionally clear the generated value using a non-public value.
		s.value = nil
	}
}
