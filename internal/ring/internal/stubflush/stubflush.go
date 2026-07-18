//go:build cgo

// Package stubflush provides an empty definition of the C ring-flush symbol
// so that the ring package's unit tests can link without pulling in the
// engine (gd.c and the startup callbacks). It must only ever be imported by
// tests that substitute the ring's dispatch function: importing it alongside
// the real engine would define the symbol twice.
package stubflush

// #include <stdint.h>
// void gd_ring_flush(void *entries, uint32_t tail, uint32_t head, uint32_t *crash_index) {}
import "C"
