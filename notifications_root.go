package gd

// #include "gd.h"
import "C"

import (
	"graphics.gd/internal/notifyfilter"
)

// Arm the notification-filter bridge: when classdb registers a class that
// implements a Notification handler, the engine-side filter in gd.c (which
// drops per-frame process-tick notifications before they cross into Go) must
// be switched off. Registration can happen from another package's init before
// this one runs, so check the latch as well.
func init() {
	notifyfilter.Disable = func() {
		C.gd_go_handles_notifications = true
	}
	if notifyfilter.Wanted() {
		notifyfilter.Disable()
	}
}
