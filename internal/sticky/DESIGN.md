# Sticky-P inbound dispatch

## Goal
Eliminate the per-call `runtime.cgocallback` transition on the inbound virtual
path (`_process`/`_physics_process`, ~20k calls/frame in spritebench). Profiling
shows the syscall-state transitions (`reentersyscall` ≈12.5ns + `exitsyscall`
≈10.5ns ≈ **23ns of the ~35ns** per call) are the largest removable component.

## Mechanism
The engine calls `extension_instance_called` (gd.c:771, registered as
`call_virtual_with_data_func`) per virtual → `gd_on_extension_instance_called`
(cgo //export) → `crosscall2` → `runtime.cgocallback` → `On.Extension.Instance.
Called`. The transition lives in cgocallback.

Sticky-P replaces that entry with a thunk that acquires a P **once per frame's
virtual batch** and keeps it live across the engine-C gaps between calls:

- **Cold path** (first call of the batch, or after STW stole the P): establish
  g+P via the proven direct-cgocallback path (resolved `runtime.cgocallback`;
  identical to runtime.link `native.MakeCgocallbackDirect`, tested), but **skip
  the release on return** — keep the P (sticky).
- **Fast path** (P still held, no STW pending): switch to the sticky goroutine
  stack, run `Dispatch`, switch back — **no exitsyscall/entersyscall**.
- **Teardown** (`EndFrame`, from `on_every_frame`): release the P so inter-frame
  engine work runs with STW unblocked.

## Why it does not deadlock (given bounded release)
Stock cgocallback drops the P to `_Psyscall` on return, which STW may *steal*.
Sticky keeps it out of `_Psyscall` across gaps, so STW can neither steal it nor
wait-to-safepoint (thread is in engine C). Two rules convert "hang" → "bounded
delay": (1) always release at the frame boundary → ≤ 1 frame; (2) check
`gcWaiting` at each call entry, take the cold/real path if set → ≤ 1 call-gap.
Unsupported on the sticky path: a `_process` that blocks on a cross-goroutine
wait (could sit behind the very STW it's delaying).

## Three unknowns to confirm before enabling the fast path
1. **`mOffsetP`** (sticky.go) — offset of `m.p` in `runtime.m` for this go1.26
   build. `g.m` is offset 48 (stable, per threadcheck); `m.p` must be verified.
2. **`gcWaitingImpl`** (sticky.go) — read `runtime.sched.gcwaiting`. Currently
   returns `true` (always-safe: forces the stock path) so wiring can be proven
   before the fast path is armed.
3. **Fast-path stack switch + `releaseStickyP`** (sticky_amd64.s) — the lean
   cgocallback-variant reuse and the teardown. Not yet written; stubbed.

## Wiring (not yet applied — keep stock entry until validated)
- classdb init sets `sticky.Dispatch` = the `On.Extension.Instance.Called` body.
- `on_every_frame` handler calls `sticky.EndFrame()`.
- gd.c registers the thunk address as `call_virtual_with_data_func` **only when
  `sticky.Enabled`**; else keeps `extension_instance_called`.

## Validation (cheap, no benchmark harness / quiet machine needed)
`go test -run x -bench BenchmarkVirtualCallback ./internal/` drives exactly this
path in-process (seconds). Plus the full suite for correctness. Only then flip
the fast path on and A/B on the spritebench harness.
