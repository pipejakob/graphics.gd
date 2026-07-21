// Package notifyfilter coordinates the engine-side filter that drops per-frame
// process-tick notifications (NOTIFICATION_PROCESS and friends) before they
// cross into Go. The filter is active by default; classdb calls [Want] when a
// registered class implements a Notification handler, which disables it so
// every notification is delivered. The root graphics.gd package (which owns
// gd.c) registers [Disable]; this indirection exists because classdb cannot
// import the root package. Package initialisation order between the two is not
// guaranteed, so both sides handle running first: Want latches, Disable's
// registrar checks the latch.
package notifyfilter

var (
	// Disable turns the engine-side filter off. Set by the graphics.gd root
	// package init; nil on targets with no C bridge (e.g. wasm).
	Disable func()

	wanted bool
)

// Want records that some registered class handles raw engine notifications and
// disables the engine-side filter if the bridge is armed.
func Want() {
	wanted = true
	if Disable != nil {
		Disable()
	}
}

// Wanted reports whether Want has been called; used by the root package in
// case a class registered before its init ran.
func Wanted() bool {
	return wanted
}
