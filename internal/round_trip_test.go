package gd_test

import (
	"reflect"
	"testing"

	gd "graphics.gd/internal"
	"graphics.gd/internal/pointers"
	"graphics.gd/variant"
)

// TestConvenientRoundTrip asserts that values passed through the engine as
// variants round-trip back into Go code as their convenience types when they
// arrive through an `any` interface, as reported in
// https://github.com/quaadgras/graphics.gd/discussions/313 where signal
// callbacks like RichTextLabel.OnMetaClicked received raw gd.Variant values.
func TestConvenientRoundTrip(t *testing.T) {
	runOnMain(t, func(t testing.TB) {
		var got any
		callable := gd.NewCallable(func(meta any) { got = meta })
		callable.Call(gd.NewVariant("stat_name"))
		if s, ok := got.(string); !ok || s != "stat_name" {
			t.Fatalf("expected string %q, got %[2]T %[2]v", "stat_name", got)
		}
		callable.Call(gd.NewVariant(22))
		if i, ok := got.(int); !ok || i != 22 {
			t.Fatalf("expected int 22, got %[1]T %[1]v", got)
		}
		callable.Call(gd.NewVariant(2.5))
		if f, ok := got.(float64); !ok || f != 2.5 {
			t.Fatalf("expected float64 2.5, got %[1]T %[1]v", got)
		}
		callable.Call(gd.NewVariant([]any{1, "two"}))
		if a, ok := got.([]any); !ok || !reflect.DeepEqual(a, []any{1, "two"}) {
			t.Fatalf("expected []any{1, \"two\"}, got %[1]T %[1]v", got)
		}
		callable.Call(gd.NewVariant(map[string]string{"hello": "world"}))
		if m, ok := got.(map[any]any); !ok || !reflect.DeepEqual(m, map[any]any{"hello": "world"}) {
			t.Fatalf("expected map[any]any{\"hello\": \"world\"}, got %[1]T %[1]v", got)
		}
		callable.Call(gd.NewVariant([]byte{1, 2, 3}))
		if b, ok := got.([]byte); !ok || !reflect.DeepEqual(b, []byte{1, 2, 3}) {
			t.Fatalf("expected []byte{1, 2, 3}, got %[1]T %[1]v", got)
		}
	})
}

// TestAdvancedRoundTrip asserts that engine-backed [variant.Any] values are
// extracted back into their underlying gd.Variant without being converted
// through their convenience representation, while their Interface method
// unwraps to convenience types.
func TestAdvancedRoundTrip(t *testing.T) {
	runOnMain(t, func(t testing.TB) {
		v := gd.NewVariant("hello")
		wrapped := variant.Implementation(gd.VariantProxy{}, pointers.Pack(v))
		if s, ok := wrapped.Interface().(string); !ok || s != "hello" {
			t.Fatalf("expected string %q, got %[2]T %[2]v", "hello", wrapped.Interface())
		}
		if gd.InternalVariant(wrapped) != v {
			t.Fatal("expected InternalVariant to extract the original variant")
		}
		local := variant.New("local")
		extracted := gd.InternalVariant(local)
		if extracted.Type() != gd.NewVariant("local").Type() {
			t.Fatalf("expected a String variant, got %v", extracted.Type())
		}
		if s, ok := extracted.ConvenientInterface().(string); !ok || s != "local" {
			t.Fatalf("expected %q, got %q", "local", s)
		}
	})
}
