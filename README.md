# numfmt

Reusable Go library for compact SI-style quantization of wide-range positive values.

## Supported bit widths

- `8`, `16`, `32` total bits
- Configurable split between exponent and mantissa areas
- Configurable radix base (default examples use `1000`)
- Optional range-constrained mode (`min` + `max`) to reduce quantization in known operating windows

## Default format

`DefaultCodec` matches the original behavior:

- `16` total bits
- `3` exponent bits
- `13` mantissa bits
- `base = 1000`

## Install

```bash
go get numfmt
```

## Usage (default 16-bit codec)

```go
package main

import (
	"fmt"
	numfmt "numfmt"
)

func main() {
	code := numfmt.Encode(12_345_678.9)
	approx := numfmt.Decode(code)
	fmt.Println(code, approx)
}
```

## Custom codec (variable exponent/mantissa)

```go
codec, err := numfmt.New(numfmt.Bits32, 6, 1000) // 6 exponent bits, 26 mantissa bits
if err != nil {
	panic(err)
}

code := codec.Encode32(42_000_000)
value := codec.Decode32(code)
_ = value
```

## Auto exponent sizing (from max input)

```go
expBits, err := numfmt.RecommendedExpBits(numfmt.Bits16, 1000, 100_000_000)
if err != nil {
	panic(err)
}

codec := numfmt.MustNew(numfmt.Bits16, expBits, 1000)
```

## Binary payload helper (header + packed values)

```go
codec := numfmt.MustNewWithRange(numfmt.Bits16, 2, 1000, 1, 2_419_200)
values := []float64{60, 3600, 30_780}

blob, err := numfmt.EncodeValuesBinary(codec, values)
if err != nil {
	panic(err)
}

restoredCodec, restoredValues, err := numfmt.DecodeValuesBinary(blob)
if err != nil {
	panic(err)
}

_ = restoredCodec
_ = restoredValues
```

Header contains total bits, exponent bits, base, range min/max, and value count.

## Range-constrained codec (lower quantization in a known domain)

```go
codec, err := numfmt.NewWithRange(numfmt.Bits16, 3, 1000, 10_000, 200_000)
if err != nil {
	panic(err)
}

// Values inside [10_000, 200_000] get finer effective precision
// because all non-zero codes are allocated to this range.
code := codec.Encode16(42_000)
value := codec.Decode16(code)
_ = value
```

## Generic encode/decode

```go
codec := numfmt.MustNew(numfmt.Bits32, 6, 1000)
code := codec.Encode(123_456_789) // uint64
value := codec.Decode(code)
_ = value
```

## Generate a quantization guide file

```bash
go test -run TestGenerateQuantizationGuide
```

This writes `docs/quantization_guide.md` with profiles from simplistic to options-galore,
including exponent explanation, mantissa detail levels, and sampled quantization error.
