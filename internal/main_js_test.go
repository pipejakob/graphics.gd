//go:build js

package gd_test

import (
	"os"
	"testing"

	"graphics.gd/variant/Callable"
)

// TestMain gates the suite on engine readiness. On js the test binary's main
// function starts running as soon as the Go runtime does, which can be before
// the engine has finished initialising: the first test would race startup and
// crash inside its first engine call (functions not yet bound on the wasm
// side). A deferred callable only executes once the engine is servicing
// frames, so waiting for one is the readiness barrier. Ordinarily user
// programs are gated the same way by startup.Scene/LoadingScene, which the
// test binary does not call.
func TestMain(m *testing.M) {
	ready := make(chan struct{})
	Callable.Defer(Callable.New(func() { close(ready) }))
	<-ready
	os.Exit(m.Run())
}
