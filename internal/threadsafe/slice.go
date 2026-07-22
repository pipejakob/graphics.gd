package threadsafe

import (
	"iter"
	"sync/atomic"
)

const (
	sliceChunkBits = 8
	sliceChunkSize = 1 << sliceChunkBits
	sliceChunkMask = sliceChunkSize - 1
)

type sliceChunk[T any] [sliceChunkSize]atomic.Pointer[T]

// Slice is a lock-free growable slice. Elements are stored behind per-slot
// atomic pointers inside fixed-size chunks, so reads and writes of existing
// indices are O(1) and never copy the backing storage — bursts of SetIndex
// calls (e.g. mass handle frees) must not reallocate per write, as that
// previously ran wasm builds into the 4 GiB linear-memory ceiling. Only the
// small chunk directory is copied (on growth), and published chunks are
// immutable, which keeps readers race-free.
type Slice[T any] struct {
	chunks atomic.Pointer[[]*sliceChunk[T]]
	length atomic.Int64
}

// grow returns the storage for index, extending the chunk directory if needed.
func (s *Slice[T]) grow(index int) *atomic.Pointer[T] {
	which, inside := index>>sliceChunkBits, index&sliceChunkMask
	for {
		oldp := s.chunks.Load()
		var old []*sliceChunk[T]
		if oldp != nil {
			old = *oldp
		}
		if which < len(old) {
			return &old[which][inside]
		}
		next := make([]*sliceChunk[T], which+1)
		copy(next, old)
		for i := len(old); i <= which; i++ {
			next[i] = new(sliceChunk[T])
		}
		s.chunks.CompareAndSwap(oldp, &next)
	}
}

// slot returns the storage for an existing index (panics if out of range,
// matching the bounds check of a plain slice).
func (s *Slice[T]) slot(index int) *atomic.Pointer[T] {
	return &(*s.chunks.Load())[index>>sliceChunkBits][index&sliceChunkMask]
}

func (s *Slice[T]) Append(value T) int {
	index := int(s.length.Add(1)) - 1
	s.grow(index).Store(&value)
	return index
}

func (s *Slice[T]) Index(index int) T {
	if p := s.slot(index).Load(); p != nil {
		return *p
	}
	var zero T
	return zero
}

func (s *Slice[T]) SetIndex(index int, value T) {
	s.slot(index).Store(&value)
}

func (s *Slice[T]) Values() iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		length := int(s.length.Load())
		for i := 0; i < length; i++ {
			if !yield(i, s.Index(i)) {
				return
			}
		}
	}
}
