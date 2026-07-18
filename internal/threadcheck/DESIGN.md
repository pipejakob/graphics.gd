# FFI Command Buffer Design

## Overview

Replace direct per-call cgo crossings with a ring buffer (command buffer) that
records engine calls and flushes them in a single cgo crossing. Combined with
a fast main-thread check, this enables automatic cross-thread dispatch and
batched execution.

## Fast Main-Thread Detection

Implemented in `internal/threadcheck`. Uses the Go runtime's dedicated g
register (R14 on amd64, R28 on arm64) to compare the current goroutine pointer
against the one captured at init time. Since the main goroutine is always
`LockOSThread`'d, its g pointer is stable for the process lifetime.

- **amd64**: `MOVQ R14, ret+0(FP)` — one instruction
- **arm64**: `MOVD g, ret+0(FP)` — one instruction
- **wasm**: always returns true (single-threaded)
- **other**: falls back to `gdextension.Host.Threads.Main()` via cgo

Cost: ~1-3 cycles. No syscall, no cgo, no TLS indirection.

## Ring Buffer Architecture

### Entry Layout

Each entry is self-contained:

    object   uintptr          — Godot object pointer
    method   uintptr          — method bind pointer
    shape    uint64           — shape-encoded argument sizes
    args     [ARGS_SIZE]byte  — copied argument data
    result   [RESULT_SIZE]byte — result written here during flush
    refs     [16]uint16       — intra-buffer references (0 = literal arg,
                                 N = use entries[N-1].result)
    owner    *uintptr         — Go-side pointer to off-load result into

### Ring Structure

    head     uint32   — next write position
    tail     uint32   — next flush position
    entries  [SIZE]Entry

Main-thread ring is SPSC (single producer = locked main goroutine, single
consumer = same goroutine during flush). Cross-thread ring is MPSC (multiple
goroutines CAS head forward, main thread drains during flush).

## Pointer Integration

Pointers to Godot objects are opaque to Go — they are never dereferenced, only
passed to engine calls. This means a pointer can be in one of two states:

    bit 0 = 0  →  real Godot pointer (always aligned, bit 0 is free)
    bit 0 = 1  →  ring index (value >> 1 = entry index)

A ring-tagged pointer is a "virtual" pointer whose actual value hasn't been
materialized yet. It exists only as a future result in the ring buffer.

### Passing Ring-Tagged Pointers

When a ring-tagged pointer is passed as an argument to another buffered call,
the entry records a ref (refs[N] = ring index). During flush, the C-side loop
resolves refs by pointing directly at the referenced entry's result slot:

    void ring_flush(ring_buffer *ring, uint64_t through) {
        for (i = ring->tail; i <= through; i++) {
            ring_entry *e = &ring->entries[i & RING_MASK];
            void *points[16];
            prepare_callframe(1, &points[0], e->shape, e->args);
            for (j = 0; j < 16; j++) {
                if (e->refs[j] != 0)
                    points[j] = ring->entries[(e->refs[j]-1) & RING_MASK].result;
            }
            gdextension_object_method_bind_ptrcall(e->method, e->object, points, e->result);
            if (e->owner != NULL)
                *e->owner = *(uintptr_t *)e->result;
        }
        ring->tail = through + 1;
    }

### Reclamation (Write Head Wraps)

When the write head wraps around to a slot that has an unmaterialized result,
the flush processes that entry and writes the real pointer back into the
Go-side pointer via the owner back-pointer. The tag bit is cleared and all
future uses of that pointer are direct.

## Flush Triggers

Go code never observes raw Godot pointers directly. Reference type returns
(Object, String, Ref, etc.) are always ring-tagged. This means:

- **Reference type return** → buffer, return ring-tagged pointer. NO FLUSH.
- **Passing a ring-tagged pointer to another call** → buffer with ref. NO FLUSH.
- **Write head wraps to occupied slot** → off-load that entry. PARTIAL FLUSH.
- **Frame boundary** → flush everything. ONE cgo crossing.
- **Concrete value type return that Go code observes** → FLUSH (the only
  mid-frame flush in typical usage).

## Deferred Value Types (Future)

For the advanced API, even value type returns (int, float, Vector2, etc.) can
be deferred using pointer-like wrappers. Go code only forces a flush when it
actually inspects the value (e.g., uses it in an if statement or Go-side
arithmetic).

Engine-side arithmetic operations (e.g., Vector2.Add) can themselves be
buffered, keeping the entire computation in the ring:

    pos := node.GetPosition()                         // ring, deferred Vector2
    node.SetPosition(pos.WithY(pos.Y.Add(1.0)))       // ring, all deferred
    length := s.Length()                               // ring, deferred int
    array.Resize(length)                               // ring, refs length

Zero mid-frame flushes. The only "pipeline stall" is when Go code needs to
branch on or compute with a concrete value from the engine.

## Cross-Thread Dispatch

**Status: implemented** — `ring.Threads` in `internal/ring/mpsc.go`, routed
from `noescape.Call` (and `jumponly.Call`, which defers to it off-main), with
variadic variant calls dispatched as whole-call thunks via `ring.Threads.Run`
and off-thread destructors queued FIFO behind their uses via
`ring.Threads.Defer`. The main thread drains the ring every frame
(`startup/garbage_collector.go`). Multi-goroutine engine tests live in
`internal/threads_test.go`; pure concurrency tests in
`internal/ring/mpsc_test.go`.

Using `threadcheck.Main()` (~1-3 cycles), calls from non-main goroutines are
routed to a separate MPSC ring. The main thread drains this ring during flush.

- **Off-thread void call** → push to MPSC ring (fire-and-forget)
- **Off-thread call needing result** → push to MPSC ring + block until
  entry status is DONE

Results are delivered in place: the return value stays in the ring entry and
the blocked goroutine reads it back directly — no per-call allocation and no
copy by the drain. Each slot moves through a per-slot sequence lifecycle
(free → published → executed → released, one generation per lap) so that the
main thread never waits on a goroutine: the drain marks a blocked caller's
slot executed and moves on; the caller releases the slot after reading its
result. An executed-but-unread slot only stalls the one producer that wraps
a full lap of the ring back onto it, which is the backpressure working as
intended.

Lifecycle notes:

- **GC frees queue behind uses.** Ring entries hold raw engine pointers
  (copied bytes, not Go references), so a dropped wrapper's engine value
  must not be freed before its buffered calls run. Destructor paths reached
  from goroutines (`noescape.Free`, the off-thread `gdreference.OwnObject`
  cleanup) enqueue the free via `ring.Threads.Defer`: the wrapper was
  necessarily alive when its calls were buffered, so FIFO order runs the
  free after every use. (A `Callable.Defer` free would instead run at the
  next `Callable.Cycle`, which can precede the frame drain.)
- **Shutdown poisons the ring.** At engine exit (`ring.Threads.Close`), the
  remaining buffered calls are drained while the engine is still alive, then
  the ring closes: goroutines parked on it wake up, and later cross-thread
  calls become no-ops with zero results instead of parking forever with
  nothing left to drain them.
- **Thunk panics don't jam the ring.** A panic inside a `Run` thunk is
  captured by the drain and re-raised on the goroutine that made the call
  (as a direct call would have); a panic inside a deferred thunk propagates
  on the main thread after the cursor has advanced past it, so the ring
  stays consistent either way.
- **Result slots are zeroed before dispatch.** Godot's ptrcall writes a
  call's return value through `T::operator=`, which unrefs whatever the
  destination bytes appear to point at — it assumes a default-constructed
  destination. Ring entries are reused every lap, so `gd_ring_flush` zeroes
  the result slot before each ptrcall; without this, stale result bytes
  from an earlier lap were unref'd, freeing a CoW value (StringName,
  NodePath, Array, …) that the earlier caller had copied out and still
  owned (intermittent SIGSEGV in `StringName::unref` / silently emptied
  values under sustained cross-thread result traffic).

### Reference-Type Lifetimes Off the Main Thread

Wrappers tracked by the `pointers` tables are frame-temporaries: the main
thread's per-frame `pointers.Cycle` expires and frees any entry not touched
for two cycles. That model is only sound for code whose lifetime is aligned
to frames — the main thread. Goroutines (and engine-owned threads) hold
values on their own timeline, so for them every proxy-wrapped engine value
(String, StringName, NodePath, Array, Dictionary, Packed\*, Variant,
Callable, Signal) is **anchored** instead: constructed pinned (invisible to
Cycle) with a `runtime.AddCleanup` anchor carried inside the proxy value,
so the Go garbage collector governs its lifetime. The cleanup releases the
engine value through `noescape.Free`'s cross-thread path — a `Defer` in the
dispatch ring, FIFO behind any still-buffered uses of the value.

All wrap sites go through the `gd.Wrap*` helpers (`internal/anchors.go`),
including the generated classdb bindings (`gdtype.go` emits them): on the
main thread they produce plain tracked frame-temporary state, elsewhere
anchored state. Anchors are additionally kept alive in a two-generation
epoch list rotated by the frame drain (`gd.CycleAnchors`), covering internal
conversions that extract a raw handle from a freshly-built proxy and drop
the proxy before the buffered call using it is even pushed.

Supporting invariants:

- `pointers.Cycle` runs exactly once per frame, in the `EveryFrame`
  callback immediately after the ring drain (the MainLoop `Process`
  callbacks no longer cycle). Two cycles with no drain between them would
  let a tracked temporary expire *and* free before the drain that executes
  its buffered use.
- `Cycle`'s free decision is atomic against concurrent readers: it condemns
  an inactive entry with a CAS on the revision word before freeing, and
  `Get`/`Bad`/`Ask` must win their activation CAS on the same word before
  returning a pointer. A racing reader either rescues the value (the
  condemn fails) or observes the closed revision and panics — it can never
  be handed a pointer the cycle is about to free.
- Values created *on the main thread* remain frame-temporaries even if a
  goroutine later uses them; cross-thread sharing of main-created wrappers
  follows the documented main-thread lifetime rules. A goroutine stalled
  for multiple frames *inside* an internal conversion (between creating a
  tracked temp and reading it back) can still observe an invalid-reference
  panic — narrowed to a clean panic by the condemn CAS, never a
  use-after-free.

### Engine-Owned Threads

Not every non-main thread may be queued: threads on which the engine calls
into Go (the resource-loading thread, WorkerThreadPool threads) must make
their engine calls directly — the engine may be blocked waiting on them, so
queueing would deadlock. `threadcheck.Mark()`, called on entry to every
engine→Go callback, registers such threads; `threadcheck.Engine()` reports
them and they bypass the ring.

A callback does not mark its thread when it is re-entrant from a
Go-initiated engine call (e.g. a binding-created callback firing inside
`Objects.Make` on a user goroutine): the generated `gdextension.Host`
bindings bracket every crossing with `threadcheck.EnterCall`/`LeaveCall`,
and `Mark` ignores callbacks that arrive between them. Without this, a user
goroutine's first object construction would permanently mis-classify its OS
thread as engine-owned and unsafe direct calls would resume.

### Thread-Safe Singletons

Methods of engine singletons that the engine itself guards against
concurrent access never need the ring: their generated bindings call
`noescape.CallThreadSafe` (or `CallThreadSafeIf` for the settings-gated
physics servers), which crosses into the engine directly from any thread.
The set is `gdfunc.ThreadSafeSingletons`, following Godot's "Thread-safe
APIs" documentation: RenderingServer (all off-render-thread calls go through
its internal locked command queue; the "unsafe" thread model is unsupported
in Godot 4), NavigationServer2D/3D, ResourceLoader, ResourceSaver,
WorkerThreadPool, and PhysicsServer2D/3D only when the corresponding
`physics/*/run_on_separate_thread` project setting is enabled (read once at
startup, see `startup/singletons.go`).

Note that a direct singleton call from a goroutine can overtake the same
goroutine's earlier, still-queued scene-tree calls; Godot itself gives no
cross-thread ordering guarantee between scene and server APIs either.

### MPSC Ring Full: Backpressure

When the MPSC ring is full, off-thread goroutines must park until the main
thread drains space. Use an atomic fast path with a sync.Cond slow path:

    type MPSCRing struct {
        head    atomic.Uint32
        tail    atomic.Uint32
        cond    sync.Cond
        entries [SIZE]Entry
    }

    func (r *MPSCRing) Claim() *Entry {
        for {
            head := r.head.Load()
            tail := r.tail.Load()
            if head-tail >= SIZE {
                // slow path: ring full, park until main thread drains
                r.cond.L.Lock()
                for r.head.Load()-r.tail.Load() >= SIZE {
                    r.cond.Wait()
                }
                r.cond.L.Unlock()
                continue
            }
            if r.head.CompareAndSwap(head, head+1) {
                return &r.entries[head&MASK]
            }
        }
    }

The main thread, after draining entries during flush:

    func (r *MPSCRing) Flush() {
        // ... process entries in C ...
        r.tail.Store(newTail)
        r.cond.Broadcast()
    }

The fast path (ring not full, CAS succeeds) touches no mutex — just two
atomic loads and a CAS. Only when the ring is actually full does a goroutine
park on the sync.Cond.

Note: the SPSC main-thread ring never has this problem. The main goroutine is
both writer and flusher, so when the ring fills up it just flushes inline.

### Fundamental Constraint

If the main thread is inside the engine (running Godot's frame tick), it
cannot drain the MPSC ring. Off-thread goroutines are blocked until the next
Go callback or frame boundary where the main thread flushes. This is inherent
— Godot's API isn't thread-safe, so off-thread calls must wait for the main
thread regardless. The ring makes the waiting explicit and batched rather than
per-call.

### Follow-Up Window (avoiding one-call-per-frame starvation)

A single drain releases every goroutine blocked at that moment, but a
goroutine making *sequential* result calls re-queues immediately after each
release — and would otherwise complete exactly one call per frame, which on
an idle (vsynced or fps-capped) main thread means one call per 16ms. The
frame-boundary drain (`ring.Threads.FlushFrame`) therefore keeps polling for
follow-up calls after it has released blocked goroutines, for a window sized
from the measured frame period: an EMA of the time between frame drains,
divided by `FollowUpRatio` (default 1/8th of the frame), clamped to
[`FollowUpMin`, `FollowUpMax`]. The deadline is fixed per frame, so a
goroutine that never stops calling cannot extend the frame beyond the
window; a drain that releases nobody costs nothing. Because the frame
callback runs before the engine's frame-delay sleep, a capped or vsynced
game pays for the window out of time it would have slept anyway.

## Example: Typical Frame

    Go callback:
      s := String.New("hello")        // entry 0, ring-tagged pointer
      node.SetName(s)                 // entry 1, refs[0] = entry 0
      node.SetVisible(true)           // entry 2, plain void
      pos := node.GetPosition()       // entry 3, ring-tagged (or deferred)
      node.SetPosition(pos)           // entry 4, refs entry 3
      // return to engine

    Flush (one cgo crossing):
      process entries 0-4 in C loop
      off-load any remaining owners
      advance tail to 5

    Result: 1 cgo crossing instead of 5.
