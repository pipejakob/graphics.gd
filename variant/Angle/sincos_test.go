package Angle

import (
	"math"
	"testing"
)

// TestSincos32 checks the polynomial against the standard library, which is an
// independent implementation at higher precision. The tolerance is a couple of
// float32 ULP: sincos32 evaluates in float64 and rounds once at the end, so it
// should agree with a correctly-rounded reference to within the rounding.
func TestSincos32(t *testing.T) {
	const tolerance = 1e-6
	for x := float32(-20); x < 20; x += 0.0009765625 {
		gotSin, gotCos := sincos32(x)
		wantSin, wantCos := math.Sincos(float64(x))
		if d := math.Abs(float64(gotSin) - wantSin); d > tolerance {
			t.Fatalf("sin(%v) = %v, want %v (off by %v)", x, gotSin, wantSin, d)
		}
		if d := math.Abs(float64(gotCos) - wantCos); d > tolerance {
			t.Fatalf("cos(%v) = %v, want %v (off by %v)", x, gotCos, wantCos, d)
		}
	}
}

// TestSincos32NonFinite pins the behaviour the assembly had: anything that is
// not a finite number has no meaningful sine or cosine, and both come back NaN
// rather than a plausible-looking value.
func TestSincos32NonFinite(t *testing.T) {
	for _, x := range []float32{
		float32(math.NaN()),
		float32(math.Inf(1)),
		float32(math.Inf(-1)),
	} {
		sin, cos := sincos32(x)
		if !math.IsNaN(float64(sin)) || !math.IsNaN(float64(cos)) {
			t.Fatalf("sincos32(%v) = (%v, %v), want (NaN, NaN)", x, sin, cos)
		}
	}
}
