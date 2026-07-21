//go:build go1.26 && amd64 && cgo && !O0

package startup

import "graphics.gd/internal/sticky"

// stickyEndFrame releases any sticky-P held across this frame's virtual-call
// batch (see internal/sticky). Inert until the sticky entry thunk is armed.
func stickyEndFrame() { sticky.EndFrame() }
