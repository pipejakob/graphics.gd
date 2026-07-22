package threadsafe

import (
	"sync"
	"testing"
)

func TestSliceSemantics(t *testing.T) {
	var s Slice[[2]any]
	for i := range 1000 {
		if got := s.Append([2]any{i, i}); got != i {
			t.Fatalf("Append returned %d, want %d", got, i)
		}
	}
	s.SetIndex(517, [2]any{"x", "y"})
	if got := s.Index(517); got != ([2]any{"x", "y"}) {
		t.Fatalf("Index(517) = %v", got)
	}
	if got := s.Index(999); got != ([2]any{999, 999}) {
		t.Fatalf("Index(999) = %v", got)
	}
	count := 0
	for i, v := range s.Values() {
		if i != count {
			t.Fatalf("Values index %d, want %d", i, count)
		}
		if i == 517 {
			if v != ([2]any{"x", "y"}) {
				t.Fatalf("Values(517) = %v", v)
			}
		}
		count++
	}
	if count != 1000 {
		t.Fatalf("Values yielded %d elements, want 1000", count)
	}
}

// SetIndex must not copy the backing storage: bursts of handle frees
// previously reallocated the entire slice per write, running wasm builds
// into the 4 GiB linear-memory ceiling.
func TestSetIndexDoesNotCopy(t *testing.T) {
	var s Slice[[2]any]
	for i := range 40_000 {
		s.Append([2]any{i, i})
	}
	allocs := testing.AllocsPerRun(100, func() {
		s.SetIndex(20_000, [2]any{})
	})
	// One boxed value per write; the old copy-per-write implementation
	// allocated the whole 40k-element array (~1.3 MB) here.
	if allocs > 2 {
		t.Fatalf("SetIndex allocates %v objects per call, want <= 2", allocs)
	}
}

func TestSliceConcurrent(t *testing.T) {
	var s Slice[[2]any]
	for i := range sliceChunkSize * 4 {
		s.Append([2]any{i, i})
	}
	var wg sync.WaitGroup
	for w := range 4 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for i := range 10_000 {
				s.SetIndex((w*31+i)%(sliceChunkSize*4), [2]any{i, i})
			}
		}()
		go func() {
			defer wg.Done()
			for i := range 10_000 {
				v := s.Index((w*17 + i) % (sliceChunkSize * 4))
				if v[0] != v[1] {
					panic("torn read")
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 100 {
			s.Append([2]any{-1, -1})
		}
	}()
	wg.Wait()
}
