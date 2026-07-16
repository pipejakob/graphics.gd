package variant_test

import (
	"fmt"
	"testing"

	"graphics.gd/variant"
)

func TestAny(t *testing.T) {
	var i16 = variant.New(int16(42))
	if i16.Interface() != int16(42) {
		t.Error("i16.Interface() != 42")
	}
	var hello = variant.New("hello")
	if hello.Interface() != "hello" {
		fmt.Println(hello.Interface())
		t.Error("hello.Interface() != 'hello'")
	}
	var bytes = variant.New([]byte{0x01, 0x02, 0x03})
	if fmt.Sprintf("%v", bytes.Interface()) != "[1 2 3]" {
		t.Error("bytes.Interface() != [1 2 3]")
	}

	var i8 = variant.New(int8(22))
	if i8.Int8() != int8(22) {
		t.Error("i8.Int8() != 22")
	}
}

// TestNumericGetters asserts that the numeric getters tolerate any width or
// signedness the value was stored with, rather than panicking unless the
// stored type matches exactly.
func TestNumericGetters(t *testing.T) {
	if variant.New(int8(1)).Int64() != 1 {
		t.Error("New(int8).Int64() != 1")
	}
	if variant.New(22).Int64() != 22 {
		t.Error("New(int).Int64() != 22")
	}
	if variant.New(uint16(3)).Int() != 3 {
		t.Error("New(uint16).Int() != 3")
	}
	if variant.New(int64(4)).Int32() != 4 {
		t.Error("New(int64).Int32() != 4")
	}
	if variant.New(uint64(5)).Uint64() != 5 {
		t.Error("New(uint64).Uint64() != 5")
	}
	if variant.New(uint(6)).Uint64() != 6 {
		t.Error("New(uint).Uint64() != 6")
	}
	if variant.New(float32(2.5)).Float64() != 2.5 {
		t.Error("New(float32).Float64() != 2.5")
	}
	if variant.New(float64(1.5)).Float32() != 1.5 {
		t.Error("New(float64).Float32() != 1.5")
	}
	type NamedInt int32
	if variant.New(NamedInt(7)).Int64() != 7 {
		t.Error("New(NamedInt).Int64() != 7")
	}
	type NamedFloat float32
	if variant.New(NamedFloat(0.5)).Float64() != 0.5 {
		t.Error("New(NamedFloat).Float64() != 0.5")
	}
}

func BenchmarkNewAllocs(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		variant.New(42)
	}
}
