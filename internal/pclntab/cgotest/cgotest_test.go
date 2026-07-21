//go:build amd64 || arm64

package cgotest_test

import (
	"testing"
	"unsafe"

	"graphics.gd/internal/pclntab"
	"graphics.gd/internal/pclntab/cgotest"
)

func TestAsmcgocall(t *testing.T) {
	v := int32(1)
	ret := pclntab.Asmcgocall(cgotest.AddOnePtr(), unsafe.Pointer(&v))
	if v != 2 {
		t.Fatalf("C function did not run: arg = %d, want 2", v)
	}
	if ret != 41 {
		t.Fatalf("return value = %d, want 41", ret)
	}
}
