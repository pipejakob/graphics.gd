package pointers_test

import (
	"runtime"
	"sync"
	"testing"

	"graphics.gd/internal/pointers"
)

var simulated_pointers = make(map[[1]uint64]bool)

type MyPointer pointers.Type[MyPointer, [1]uint64]

func (ptr MyPointer) Free() {
	raw, ok := pointers.End(ptr)
	if ok {
		delete(simulated_pointers, raw)
	}
}

func BenchmarkDiscrete(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%512 == 0 {
			pointers.Cycle()
		}
		var p = pointers.New[MyPointer]([1]uint64{1})
		if pointers.Get(p) != [1]uint64{1} {
			b.Fatal("bad")
		}
	}
}

type MyString pointers.Type[MyString, [1]uint64]

func (ptr MyString) Free() {}

type MySlice pointers.Type[MySlice, [1]uint64]

func (ptr MySlice) Free() {}

var freeable_freed bool

type MyFreeable pointers.Type[MyFreeable, [1]uint64]

func (ptr MyFreeable) Free() {
	if _, ok := pointers.End(ptr); ok {
		freeable_freed = true
	}
}

// TestCycleCondemnRace races a Get rescue of an expired pointer against the
// Cycle that would free it. The two must linearize on the revision word: if
// Get returns a value, its activation CAS won and that round's Cycle must
// not have freed the entry; if the Cycle condemned it first, Get must panic
// with an invalid-reference error rather than return a pointer to freed
// memory (which is what happened when Get ignored its CAS result).
func TestCycleCondemnRace(t *testing.T) {
	for round := 0; round < 10_000; round++ {
		ptr := pointers.New[MyFreeable]([1]uint64{42})
		freeable_freed = false
		pointers.Cycle() // deterministic expire: next cycle may free it.
		var returned, panicked bool
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			defer func() {
				if recover() != nil {
					panicked = true
				}
			}()
			if pointers.Get(ptr) != [1]uint64{42} {
				t.Error("Get returned a wrong value")
			}
			returned = true
		}()
		go func() {
			defer wg.Done()
			runtime.Gosched()
			pointers.Cycle() // condemn+free, unless the Get rescued it first.
		}()
		wg.Wait()
		switch {
		case returned && freeable_freed:
			t.Fatalf("round %d: Get returned a value the racing Cycle freed", round)
		case panicked && !freeable_freed:
			t.Fatalf("round %d: Get panicked but nothing was freed", round)
		case !returned && !panicked:
			t.Fatalf("round %d: Get neither returned nor panicked", round)
		}
		if !freeable_freed {
			ptr.Free()
		}
		pointers.Cycle()
		pointers.Cycle()
	}
}

func TestPointersV2(t *testing.T) {
	pointers.New[MyPointer]([1]uint64{1})
	simulated_pointers[[1]uint64{1}] = true

	pointers.Cycle()
	pointers.Cycle()

	if len(simulated_pointers) != 0 {
		t.Fatal("simulated pointers not freed")
	}
}
