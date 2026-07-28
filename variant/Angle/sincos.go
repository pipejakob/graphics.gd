//go:build !precision_double && (amd64 || arm64) && !O0

package Angle

import "math"

// Sine and cosine for float32, by range reduction to a quadrant followed by a
// pair of polynomials. This was hand-written assembly (sin_amd64.s,
// sin_arm64.s) until it was measured against Go: the assembly was declared
// without <ABIInternal>, so every call crossed an ABI0 wrapper and passed its
// argument and results through memory, which cost more than the arithmetic
// saved. The polynomials and constants below are the assembly's, unchanged,
// and were verified to produce bit-identical results before it was deleted.
//
// The three entry points share the reduction and the polynomials but not the
// quadrant selection, which differs for each and is where a shared
// implementation would do redundant work: sin32 built on sincos32 measured
// ~19% slower than the assembly it replaced, because it evaluated the cosine's
// selection and sign only to discard them. The helpers are kept small enough
// to inline so that sharing them costs nothing.
const (
	invpio2 = 0.6366197723675814     // 2/π
	pio2hi  = 1.5707963109016418     // π/2, high word
	pio2lo  = 1.5893254773528196e-08 // π/2, low word

	sin1 = -0.16666666641626524
	sin2 = 0.008333329385889463
	sin3 = -0.0001984126982958954
	sin4 = 2.718311493989822e-06

	cos0 = -0.499999997251031
	cos1 = 0.04166662332373906
	cos2 = -0.001388676377461
	cos3 = 2.439044879627741e-05
)

// reduce maps the magnitude of x onto a quadrant and the remainder within it.
// The rounding is to nearest, not truncation: half a quadrant either way would
// pair the argument with the wrong polynomial. It reports ok false for input
// that is not a finite number, which has no meaningful sine or cosine.
func reduce(x float32) (quadrant int64, r float64, negative, ok bool) {
	bits := math.Float32bits(x)
	magnitude := bits &^ (1 << 31)
	if magnitude >= 0x7F800000 { // NaN or ±Inf
		return 0, 0, false, false
	}
	d := float64(math.Float32frombits(magnitude))
	quadrant = int64(math.RoundToEven(d * invpio2))
	n := float64(quadrant)
	return quadrant, d - n*pio2hi - n*pio2lo, bits>>31 != 0, true
}

// sinPoly and cosPoly approximate sine and cosine on the reduced argument.
// Neither depends on the other, so where both are needed the two dependency
// chains overlap.
func sinPoly(r, r2 float64) float64 {
	return ((((sin4*r2+sin3)*r2+sin2)*r2+sin1)*r2)*r + r
}

func cosPoly(r2 float64) float64 {
	return (((cos3*r2+cos2)*r2+cos1)*r2+cos0)*r2 + 1
}

// sincos32 returns the sine and cosine of x, sharing one range reduction
// between them so the pair costs close to half of computing them separately.
func sincos32(x float32) (sin, cos float32) {
	quadrant, r, negative, ok := reduce(x)
	if !ok {
		nan := float32(math.NaN())
		return nan, nan
	}
	r2 := r * r
	sr, cr := sinPoly(r, r2), cosPoly(r2)
	if quadrant&1 != 0 {
		sr, cr = cr, sr
	}
	if quadrant&2 != 0 {
		sr = -sr
	}
	if (quadrant+1)&2 != 0 {
		cr = -cr
	}
	if negative { // the magnitude mask discarded the sign; sine is odd
		sr = -sr
	}
	return float32(sr), float32(cr)
}

func sin32(x float32) float32 {
	quadrant, r, negative, ok := reduce(x)
	if !ok {
		return float32(math.NaN())
	}
	r2 := r * r
	// Both polynomials are evaluated unconditionally, as the assembly did.
	// Computing the unused one under a branch measured the same here, but
	// this benchmark sweeps the angle smoothly, so its quadrant is highly
	// predictable; a caller with unrelated angles per call — a scene full of
	// sprites, say — would pay a misprediction the sweep never shows.
	v, other := sinPoly(r, r2), cosPoly(r2)
	if quadrant&1 != 0 {
		v = other
	}
	if quadrant&2 != 0 {
		v = -v
	}
	if negative {
		v = -v
	}
	return float32(v)
}

func cos32(x float32) float32 {
	quadrant, r, _, ok := reduce(x)
	if !ok {
		return float32(math.NaN())
	}
	r2 := r * r
	v, other := cosPoly(r2), sinPoly(r, r2) // both, as in sin32
	if quadrant&1 != 0 {
		v = other
	}
	if (quadrant+1)&2 != 0 {
		v = -v
	}
	return float32(v) // cosine is even, so the sign of x does not carry
}
