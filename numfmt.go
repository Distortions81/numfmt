package numfmt

import (
	"fmt"
	"math"
)

const (
	Bits8  uint8 = 8
	Bits16 uint8 = 16
	Bits32 uint8 = 32
)

// Codec encodes/decodes wide-range positive values into a compact SI-style code.
//
// Code layout (unbounded mode):
// - code 0: exact zero (and invalid/non-positive inputs on encode)
// - high ExpBits: exponent bucket (base Base)
// - low (TotalBits-ExpBits) bits: logarithmically quantized mantissa in [1, Base)
//
// Range-constrained mode can be enabled by setting RangeMin/RangeMax via NewWithRange
// or WithRange. In that mode, all non-zero codes are distributed across [RangeMin, RangeMax]
// in log space for better precision in a bounded domain.
type Codec struct {
	TotalBits uint8
	ExpBits   uint8
	Base      float64

	RangeMin float64
	RangeMax float64
	useRange bool
}

// DefaultCodec matches the original behavior: 16-bit, 3 exponent bits, base 1000.
var DefaultCodec = MustNew(Bits16, 3, 1000)

// New validates and returns a reusable SI codec.
func New(totalBits, expBits uint8, base float64) (Codec, error) {
	if !isValidTotalBits(totalBits) {
		return Codec{}, fmt.Errorf("totalBits must be one of [8,16,32], got %d", totalBits)
	}
	if expBits == 0 || expBits >= totalBits {
		return Codec{}, fmt.Errorf("expBits must be in [1,%d], got %d", totalBits-1, expBits)
	}
	if !(base > 1) || math.IsNaN(base) || math.IsInf(base, 0) {
		return Codec{}, fmt.Errorf("base must be finite and > 1, got %v", base)
	}
	return Codec{TotalBits: totalBits, ExpBits: expBits, Base: base}, nil
}

// RecommendedExpBits returns an exponent-bit count sized for maxValue in SI mode.
//
// The returned value is in [1,totalBits-1]. If maxValue is too large to represent
// without saturation for the given totalBits, this returns totalBits-1.
func RecommendedExpBits(totalBits uint8, base, maxValue float64) (uint8, error) {
	if !isValidTotalBits(totalBits) {
		return 0, fmt.Errorf("totalBits must be one of [8,16,32], got %d", totalBits)
	}
	if !(base > 1) || math.IsNaN(base) || math.IsInf(base, 0) {
		return 0, fmt.Errorf("base must be finite and > 1, got %v", base)
	}
	if !(maxValue > 0) || math.IsNaN(maxValue) || math.IsInf(maxValue, 0) {
		return 0, fmt.Errorf("maxValue must be finite and > 0, got %v", maxValue)
	}

	// Need maxExp >= floor(log_base(maxValue)) to avoid saturation in SI mode.
	requiredMaxExp := uint64(0)
	if maxValue >= 1 {
		requiredMaxExp = uint64(math.Floor(math.Log(maxValue)/math.Log(base) + 1e-12))
	}

	b := uint8(1)
	for b < totalBits && ((uint64(1)<<b)-1) < requiredMaxExp {
		b++
	}
	if b >= totalBits {
		return totalBits - 1, nil
	}
	return b, nil
}

// NewWithRange returns a codec that prioritizes precision within [minValue, maxValue].
func NewWithRange(totalBits, expBits uint8, base, minValue, maxValue float64) (Codec, error) {
	c, err := New(totalBits, expBits, base)
	if err != nil {
		return Codec{}, err
	}
	return c.WithRange(minValue, maxValue)
}

// MustNew is like New but panics on invalid arguments.
func MustNew(totalBits, expBits uint8, base float64) Codec {
	c, err := New(totalBits, expBits, base)
	if err != nil {
		panic(err)
	}
	return c
}

// MustNewWithRange is like NewWithRange but panics on invalid arguments.
func MustNewWithRange(totalBits, expBits uint8, base, minValue, maxValue float64) Codec {
	c, err := NewWithRange(totalBits, expBits, base, minValue, maxValue)
	if err != nil {
		panic(err)
	}
	return c
}

// WithRange enables range-constrained mode on a codec copy.
func (c Codec) WithRange(minValue, maxValue float64) (Codec, error) {
	if !(minValue > 0) || math.IsNaN(minValue) || math.IsInf(minValue, 0) {
		return Codec{}, fmt.Errorf("minValue must be finite and > 0, got %v", minValue)
	}
	if !(maxValue > minValue) || math.IsNaN(maxValue) || math.IsInf(maxValue, 0) {
		return Codec{}, fmt.Errorf("maxValue must be finite and > minValue, got %v", maxValue)
	}
	c.RangeMin = minValue
	c.RangeMax = maxValue
	c.useRange = true
	return c, nil
}

// RangeEnabled reports whether this codec is using range-constrained mode.
func (c Codec) RangeEnabled() bool { return c.useRange }

func isValidTotalBits(totalBits uint8) bool {
	return totalBits == Bits8 || totalBits == Bits16 || totalBits == Bits32
}

func (c Codec) mantBits() uint8 {
	return c.TotalBits - c.ExpBits
}

func (c Codec) mantMask() uint64 {
	return (uint64(1) << c.mantBits()) - 1
}

func (c Codec) maxExp() uint64 {
	return (uint64(1) << c.ExpBits) - 1
}

func (c Codec) totalMask() uint64 {
	return (uint64(1) << c.TotalBits) - 1
}

func (c Codec) maxCode() uint64 {
	return (c.maxExp() << c.mantBits()) | c.mantMask()
}

// Encode packs a non-negative value into a compact code.
//
// Behavior:
// - v <= 0, NaN => 0
// - +Inf or too large => max code (saturation)
// - finite positive values are monotonic after quantization
func (c Codec) Encode(v float64) uint64 {
	if !(v > 0) || math.IsNaN(v) {
		return 0
	}
	if math.IsInf(v, 1) {
		return c.maxCode()
	}
	if c.useRange {
		return c.encodeRanged(v)
	}
	return c.encodeSI(v)
}

func (c Codec) encodeSI(v float64) uint64 {
	mantBits := c.mantBits()
	mantMask := c.mantMask()
	maxExp := c.maxExp()

	exp := uint64(0)
	mant := v
	for mant >= c.Base && exp < maxExp {
		mant /= c.Base
		exp++
	}

	// Saturated exponent: clamp to max representable code.
	if exp == maxExp && mant >= c.Base {
		return c.maxCode()
	}
	if mant < 1 {
		mant = 1
	}
	if mant >= c.Base {
		// Guard against roundoff at bucket boundaries.
		mant = math.Nextafter(c.Base, 0)
	}

	logBase := math.Log(c.Base)
	logT := math.Log(mant) / logBase // [0,1)
	if logT < 0 {
		logT = 0
	}
	if logT > 1 {
		logT = 1
	}

	q := uint64(1)
	if mantMask > 1 {
		q = 1 + uint64(math.Round(logT*float64(mantMask-1)))
	}
	if q > mantMask {
		q = mantMask
	}
	return (exp << mantBits) | q
}

func (c Codec) encodeRanged(v float64) uint64 {
	if v <= c.RangeMin {
		return 1
	}
	if v >= c.RangeMax {
		return c.maxCode()
	}

	levels := c.maxCode()
	if levels <= 1 {
		return 1
	}

	logSpan := math.Log(c.RangeMax / c.RangeMin)
	t := math.Log(v/c.RangeMin) / logSpan
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}

	code := 1 + uint64(math.Round(t*float64(levels-1)))
	if code < 1 {
		code = 1
	}
	if code > levels {
		code = levels
	}
	return code
}

// Decode reconstructs an approximate value from a compact code.
func (c Codec) Decode(code uint64) float64 {
	if code == 0 {
		return 0
	}
	code &= c.totalMask()
	if c.useRange {
		return c.decodeRanged(code)
	}
	return c.decodeSI(code)
}

func (c Codec) decodeSI(code uint64) float64 {
	mantBits := c.mantBits()
	mantMask := c.mantMask()
	exp := (code >> mantBits) & c.maxExp()
	q := code & mantMask
	if q == 0 {
		return 0
	}

	logT := 0.0
	if mantMask > 1 {
		logT = float64(q-1) / float64(mantMask-1)
	}
	mant := math.Pow(c.Base, logT)
	return mant * math.Pow(c.Base, float64(exp))
}

func (c Codec) decodeRanged(code uint64) float64 {
	levels := c.maxCode()
	if levels <= 1 {
		return c.RangeMin
	}
	if code > levels {
		code = levels
	}
	if code < 1 {
		return 0
	}
	logSpan := math.Log(c.RangeMax / c.RangeMin)
	t := float64(code-1) / float64(levels-1)
	return c.RangeMin * math.Exp(t*logSpan)
}

// Encode8 packs using an 8-bit codec.
func (c Codec) Encode8(v float64) uint8 { return uint8(c.Encode(v)) }

// Decode8 decodes an 8-bit code.
func (c Codec) Decode8(code uint8) float64 { return c.Decode(uint64(code)) }

// Encode16 packs using a 16-bit codec.
func (c Codec) Encode16(v float64) uint16 { return uint16(c.Encode(v)) }

// Decode16 decodes a 16-bit code.
func (c Codec) Decode16(code uint16) float64 { return c.Decode(uint64(code)) }

// Encode32 packs using a 32-bit codec.
func (c Codec) Encode32(v float64) uint32 { return uint32(c.Encode(v)) }

// Decode32 decodes a 32-bit code.
func (c Codec) Decode32(code uint32) float64 { return c.Decode(uint64(code)) }

// Encode uses the default 16-bit codec.
func Encode(v float64) uint16 { return DefaultCodec.Encode16(v) }

// Decode uses the default 16-bit codec.
func Decode(code uint16) float64 { return DefaultCodec.Decode16(code) }
