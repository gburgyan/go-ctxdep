package ctxdep

import (
	"context"
	"fmt"
	"testing"
)

type optionalTestInterface interface {
	GetValue() string
}

type optionalTestImpl struct {
	Value string
}

func (t *optionalTestImpl) GetValue() string {
	return t.Value
}

func TestGetOptional(t *testing.T) {
	type TestDep struct {
		Value string
	}

	t.Run("found dependency", func(t *testing.T) {
		ctx := context.Background()
		dep := &TestDep{Value: "test"}
		ctx = NewDependencyContext(ctx, dep)

		result, found := GetOptional[*TestDep](ctx)
		if !found {
			t.Error("expected to find dependency")
		}
		if result.Value != "test" {
			t.Errorf("expected value 'test', got '%s'", result.Value)
		}
	})

	t.Run("missing dependency", func(t *testing.T) {
		ctx := context.Background()
		ctx = NewDependencyContext(ctx)

		result, found := GetOptional[*TestDep](ctx)
		if found {
			t.Error("expected not to find dependency")
		}
		if result != nil {
			t.Error("expected nil result")
		}
	})

	t.Run("interface optional", func(t *testing.T) {
		ctx := context.Background()
		impl := &optionalTestImpl{Value: "interface test"}
		ctx = NewDependencyContext(ctx, impl)

		result, found := GetOptional[optionalTestInterface](ctx)
		if !found {
			t.Error("expected to find interface implementation")
		}
		if result.GetValue() != "interface test" {
			t.Errorf("expected 'interface test', got '%s'", result.GetValue())
		}
	})
}

func TestGetBatchOptional(t *testing.T) {
	type Dep1 struct{ Value string }
	type Dep2 struct{ Value int }
	type Dep3 struct{ Value bool }

	ctx := context.Background()
	dep1 := &Dep1{Value: "test"}
	dep2 := &Dep2{Value: 42}
	// Dep3 is intentionally not added
	ctx = NewDependencyContext(ctx, dep1, dep2)

	var r1 *Dep1
	var r2 *Dep2
	var r3 *Dep3

	results := GetBatchOptional(ctx, &r1, &r2, &r3)

	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	if !results[0] {
		t.Error("expected first dependency to be found")
	}
	if r1.Value != "test" {
		t.Errorf("expected r1.Value to be 'test', got '%s'", r1.Value)
	}

	if !results[1] {
		t.Error("expected second dependency to be found")
	}
	if r2.Value != 42 {
		t.Errorf("expected r2.Value to be 42, got %d", r2.Value)
	}

	if results[2] {
		t.Error("expected third dependency to not be found")
	}
	if r3 != nil {
		t.Error("expected r3 to be nil")
	}
}

func TestOptionalWithGenerators(t *testing.T) {
	type GeneratedDep struct {
		Value string
	}

	t.Run("successful generator", func(t *testing.T) {
		ctx := context.Background()
		generator := func() *GeneratedDep {
			return &GeneratedDep{Value: "generated"}
		}
		ctx = NewDependencyContext(ctx, generator)

		result, found := GetOptional[*GeneratedDep](ctx)
		if !found {
			t.Error("expected to find generated dependency")
		}
		if result.Value != "generated" {
			t.Errorf("expected 'generated', got '%s'", result.Value)
		}
	})

	t.Run("failing generator", func(t *testing.T) {
		ctx := context.Background()
		generator := func() (*GeneratedDep, error) {
			return nil, fmt.Errorf("generation failed")
		}
		ctx = NewDependencyContext(ctx, generator)

		result, found := GetOptional[*GeneratedDep](ctx)
		if found {
			t.Error("expected not to find dependency due to generator error")
		}
		if result != nil {
			t.Error("expected nil result")
		}
	})
}
