// Package cgotest provides the C half of pclntab's end-to-end test: a real C
// function for Asmcgocall to run. It is a separate non-test package because
// cgo is not allowed in _test.go files.
package cgotest

/*
static int gd_pclntab_test_addone(void *p) {
	*(int *)p += 1;
	return 41;
}
static void *gd_pclntab_test_addone_ptr(void) {
	return (void *)gd_pclntab_test_addone;
}
*/
import "C"
import "unsafe"

// AddOnePtr returns a C function int(*)(void*) that increments the pointed-to
// int32 and returns 41.
func AddOnePtr() unsafe.Pointer { return C.gd_pclntab_test_addone_ptr() }
