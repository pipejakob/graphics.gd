package ring

import (
	"runtime"
	"sync"
	"testing"
	"time"
	"unsafe"

	_ "graphics.gd/internal/ring/internal/stubflush" // stands in for the engine's C flush (unused: dispatch is faked)

	"graphics.gd/internal/gdextension"
)

func newTestRing() *MPSC {
	r := &MPSC{}
	r.Init(new(mpscShared), new([Size]Entry))
	return r
}

// fakeDispatch replaces the engine crossing with fn for the duration of a
// test. fn observes each entry in flush order and may write its result slot.
func fakeDispatch(t *testing.T, fn func(e *Entry)) {
	t.Helper()
	old := dispatch
	dispatch = func(entries unsafe.Pointer, tail, head uint32) {
		ring := (*[Size]Entry)(entries)
		for i := tail; i != head; i++ {
			fn(&ring[i&Mask])
		}
	}
	t.Cleanup(func() { dispatch = old })
}

// drain flushes r on the calling goroutine (standing in for the main thread)
// until stop reports true, failing the test if it takes too long.
func drain(t *testing.T, r *MPSC, stop func() bool) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for !stop() {
		if time.Now().After(deadline) {
			t.Fatal("drain did not complete in time (deadlock?)")
		}
		r.Flush()
		runtime.Gosched()
	}
	r.Flush() // anything published between the last check and now
}

// TestMPSCPerGoroutineOrder floods the ring from concurrent producers, well
// past its capacity to exercise backpressure, and verifies that every entry
// arrives and that each producer's entries arrive in the order it made them.
func TestMPSCPerGoroutineOrder(t *testing.T) {
	r := newTestRing()
	type rec struct{ producer, seq uintptr }
	var got []rec
	fakeDispatch(t, func(e *Entry) {
		got = append(got, rec{e.Object, e.Method})
	})
	const producers = 8
	const entries = 1000 // producers*entries >> Size, so producers park on the full ring
	var wg sync.WaitGroup
	for p := range producers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for s := range entries {
				r.Buffer(uintptr(p+1), uintptr(s), 0, nil, 0)
			}
		}()
	}
	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	drain(t, r, func() bool {
		select {
		case <-finished:
			return true
		default:
			return false
		}
	})
	if len(got) != producers*entries {
		t.Fatalf("expected %d entries, got %d", producers*entries, len(got))
	}
	next := make(map[uintptr]uintptr)
	for _, rec := range got {
		if rec.seq != next[rec.producer] {
			t.Fatalf("producer %d: expected entry %d, got %d", rec.producer, next[rec.producer], rec.seq)
		}
		next[rec.producer] = rec.seq + 1
	}
}

// TestMPSCResults verifies that goroutines blocked on a result each receive
// the return value of their own call.
func TestMPSCResults(t *testing.T) {
	r := newTestRing()
	fakeDispatch(t, func(e *Entry) {
		*(*uint64)(unsafe.Pointer(&e.Result[0])) = uint64(e.Object) * uint64(e.Method)
	})
	const producers = 8
	const calls = 100
	var wg sync.WaitGroup
	for p := range producers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for s := range calls {
				var result uint64
				r.Call(uintptr(p+2), uintptr(s+3), 0, nil, 0, unsafe.Pointer(&result), unsafe.Sizeof(result))
				if want := uint64(p+2) * uint64(s+3); result != want {
					t.Errorf("call (%d, %d): expected %d, got %d", p+2, s+3, want, result)
					return
				}
			}
		}()
	}
	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	drain(t, r, func() bool {
		select {
		case <-finished:
			return true
		default:
			return false
		}
	})
}

// TestMPSCThunkOrder verifies that deferred thunks run on the draining
// goroutine in FIFO order with the buffered entries around them.
func TestMPSCThunkOrder(t *testing.T) {
	r := newTestRing()
	var order []uintptr
	fakeDispatch(t, func(e *Entry) {
		order = append(order, e.Method)
	})
	r.Buffer(1, 100, 0, nil, 0)
	r.Defer(func() { order = append(order, 200) })
	r.Buffer(1, 300, 0, nil, 0)
	r.Flush()
	if len(order) != 3 || order[0] != 100 || order[1] != 200 || order[2] != 300 {
		t.Fatalf("expected [100 200 300], got %v", order)
	}
}

// TestMPSCRunBlocks verifies that Run executes its thunk before returning and
// after the caller's previously buffered entries.
func TestMPSCRunBlocks(t *testing.T) {
	r := newTestRing()
	var order []uintptr
	fakeDispatch(t, func(e *Entry) {
		order = append(order, e.Method)
	})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		r.Buffer(1, 100, 0, nil, 0)
		ran := false
		r.Run(func() {
			order = append(order, 200)
			ran = true
		})
		if !ran {
			t.Error("Run returned before its thunk executed")
		}
	}()
	drain(t, r, func() bool {
		select {
		case <-finished:
			return true
		default:
			return false
		}
	})
	if len(order) != 2 || order[0] != 100 || order[1] != 200 {
		t.Fatalf("expected [100 200], got %v", order)
	}
}

// TestMPSCArguments verifies that argument bytes are copied into the ring at
// their shape-encoded size and survive until the flush.
func TestMPSCArguments(t *testing.T) {
	r := newTestRing()
	var got []uint64
	fakeDispatch(t, func(e *Entry) {
		got = append(got, *(*uint64)(unsafe.Pointer(&e.Args[0])))
	})
	const shape = uint64(0 | (gdextension.SizeInt << 4)) // one 8-byte argument
	for i := range uint64(5) {
		arg := 0xdead0000 + i
		r.Buffer(1, uintptr(i), shape, unsafe.Pointer(&arg), 0)
	}
	r.Flush()
	for i, arg := range got {
		if arg != 0xdead0000+uint64(i) {
			t.Fatalf("entry %d: expected argument %#x, got %#x", i, 0xdead0000+i, arg)
		}
	}
	if len(got) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(got))
	}
}

// TestMPSCFollowUpWindow verifies that FlushFor keeps servicing a goroutine
// making sequential blocking calls within one window, instead of completing
// only one call per drain: 100 ping-pong round trips should finish within a
// couple of windows, not a hundred.
func TestMPSCFollowUpWindow(t *testing.T) {
	r := newTestRing()
	fakeDispatch(t, func(e *Entry) {
		*(*uint64)(unsafe.Pointer(&e.Result[0])) = uint64(e.Method) + 1
	})
	const calls = 100
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for s := range calls {
			var result uint64
			r.Call(1, uintptr(s), 0, nil, 0, unsafe.Pointer(&result), unsafe.Sizeof(result))
			if result != uint64(s)+1 {
				t.Errorf("call %d: expected %d, got %d", s, s+1, result)
				return
			}
		}
	}()
	for !r.Pending() { // wait for the first call before counting windows
		runtime.Gosched()
	}
	windows := 0
	deadline := time.Now().Add(30 * time.Second)
	for {
		select {
		case <-finished:
			// without the window this takes ~one drain per call (100);
			// with it, single digits plus scheduling noise.
			if windows > calls/4 {
				t.Errorf("expected %d sequential calls to complete within a few follow-up windows, took %d", calls, windows)
			}
			return
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("ping-pong did not complete in time (deadlock?)")
		}
		r.FlushFor(100 * time.Millisecond)
		windows++
	}
}

// TestMPSCFrameWindow verifies that FlushFrame's dynamic follow-up window
// tracks the frame period and services sequential blocking calls within a
// simulated frame instead of one per frame.
func TestMPSCFrameWindow(t *testing.T) {
	r := newTestRing()
	fakeDispatch(t, func(e *Entry) {
		*(*uint64)(unsafe.Pointer(&e.Result[0])) = uint64(e.Method) + 1
	})
	// establish a ~10ms frame period (window = period/8 > wakeup latency)
	for range 8 {
		r.FlushFrame()
		time.Sleep(10 * time.Millisecond)
	}
	const calls = 50
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for s := range calls {
			var result uint64
			r.Call(1, uintptr(s), 0, nil, 0, unsafe.Pointer(&result), unsafe.Sizeof(result))
		}
	}()
	for !r.Pending() {
		runtime.Gosched()
	}
	frames := 0
	deadline := time.Now().Add(30 * time.Second)
	for {
		select {
		case <-finished:
			if frames > calls/2 {
				t.Errorf("expected %d sequential calls to complete in a few frames, took %d", calls, frames)
			}
			return
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("ping-pong did not complete in time (deadlock?)")
		}
		r.FlushFrame()
		frames++
		time.Sleep(10 * time.Millisecond)
	}
}

// TestMPSCClose verifies that closing the ring first drains what is pending
// and then turns subsequent cross-thread calls into no-ops with zero
// results, so goroutines never park on a ring that nothing drains anymore.
func TestMPSCClose(t *testing.T) {
	r := newTestRing()
	var got []uintptr
	fakeDispatch(t, func(e *Entry) {
		got = append(got, e.Method)
		*(*uint64)(unsafe.Pointer(&e.Result[0])) = 7
	})
	r.Buffer(1, 100, 0, nil, 0)
	deferred := false
	r.Defer(func() { deferred = true })
	r.Close()
	if len(got) != 1 || got[0] != 100 || !deferred {
		t.Fatalf("expected Close to drain the pending entries, got %v (deferred %v)", got, deferred)
	}
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		var result uint64
		r.Call(1, 200, 0, nil, 0, unsafe.Pointer(&result), unsafe.Sizeof(result))
		if result != 0 {
			t.Errorf("expected zero result from a closed ring, got %d", result)
		}
		ran := false
		r.Run(func() { ran = true })
		if ran {
			t.Error("expected Run to be a no-op on a closed ring")
		}
		r.Buffer(1, 300, 0, nil, 0)
		r.Defer(func() {})
	}()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("calls on a closed ring parked instead of returning")
	}
	r.Flush()
	if len(got) != 1 {
		t.Fatalf("expected no entries after close, got %v", got)
	}
}

// TestMPSCRunPanic verifies that a panic inside a Run thunk surfaces on the
// goroutine that made the call and does not jam the ring.
func TestMPSCRunPanic(t *testing.T) {
	r := newTestRing()
	fakeDispatch(t, func(e *Entry) {})
	finished := make(chan any, 1)
	go func() {
		defer func() { finished <- recover() }()
		r.Run(func() { panic("boom") })
	}()
	drain(t, r, func() bool {
		select {
		case v := <-finished:
			if v != "boom" {
				t.Errorf("expected the caller to re-panic with 'boom', got %v", v)
			}
			return true
		default:
			return false
		}
	})
	// the ring must still work afterwards
	var order []uintptr
	fakeDispatch(t, func(e *Entry) { order = append(order, e.Method) })
	r.Buffer(1, 400, 0, nil, 0)
	r.Flush()
	if len(order) != 1 || order[0] != 400 {
		t.Fatalf("ring jammed after a Run panic: %v", order)
	}
}

// TestMPSCDeferPanic verifies that a panic inside a deferred thunk
// propagates out of the flush on the draining thread, after the ring has
// been left in a consistent state.
func TestMPSCDeferPanic(t *testing.T) {
	r := newTestRing()
	var order []uintptr
	fakeDispatch(t, func(e *Entry) { order = append(order, e.Method) })
	r.Buffer(1, 100, 0, nil, 0)
	r.Defer(func() { panic("boom") })
	r.Buffer(1, 300, 0, nil, 0)
	func() {
		defer func() {
			if v := recover(); v != "boom" {
				t.Errorf("expected the flush to panic with 'boom', got %v", v)
			}
		}()
		r.Flush()
	}()
	r.Flush() // the ring must still work and resume where it left off
	if len(order) != 2 || order[0] != 100 || order[1] != 300 {
		t.Fatalf("expected [100 300] around the panicking thunk, got %v", order)
	}
}

// TestMPSCMixedStress hammers the ring with a mix of every operation from
// many goroutines at once, to be run under the race detector.
func TestMPSCMixedStress(t *testing.T) {
	r := newTestRing()
	fakeDispatch(t, func(e *Entry) {
		*(*uint64)(unsafe.Pointer(&e.Result[0])) = uint64(e.Object) + uint64(e.Method)
	})
	const producers = 16
	const rounds = 200
	var wg sync.WaitGroup
	for p := range producers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for s := range rounds {
				switch s % 4 {
				case 0, 1:
					r.Buffer(uintptr(p+1), uintptr(s), 0, nil, 0)
				case 2:
					var result uint64
					r.Call(uintptr(p+1), uintptr(s), 0, nil, 0, unsafe.Pointer(&result), unsafe.Sizeof(result))
					if want := uint64(p+1) + uint64(s); result != want {
						t.Errorf("expected %d, got %d", want, result)
						return
					}
				case 3:
					r.Defer(func() {})
				}
			}
		}()
	}
	finished := make(chan struct{})
	go func() { wg.Wait(); close(finished) }()
	drain(t, r, func() bool {
		select {
		case <-finished:
			return true
		default:
			return false
		}
	})
}
