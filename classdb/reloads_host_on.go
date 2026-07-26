//go:build reloads

package classdb

// registrationDisabled: this binary is a reloads host (see
// graphics.gd/startup with -tags reloads) — class registration is
// performed exclusively by the hot-reloadable wasm guest build of the
// project, so the host's own Register calls become no-ops.
const registrationDisabled = true
