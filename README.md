# numfmt

`numfmt` is a small Go library for compressing large positive numbers into compact integer codes with predictable error.

It is useful when raw `float64` is overkill and payload size matters.

Quick guide: https://github.com/Distortions81/numfmt/blob/Main/docs/quantization_guide.md

## Why this is useful

Many systems move huge volumes of numeric telemetry where exact values are not required:

- metrics streams (`requests/min`, latency buckets, counters)
- map and tracking data (distance, pace, durations)
- device or sensor pipelines (weights, rates, environmental readings)
- storage-heavy event logs and snapshots

In those cases, using `8/16/32` bits instead of `64` can cut storage and bandwidth significantly.

Real workload example: `1,440 records x 1,000,000 clients`.
two metrics (`hashrate` + `best share`): `16-bit` `~5.36GB` vs `int64` `~21.46GB` (`~75%` smaller)
For in-memory and for database... this is a huge help.

## Core idea

A value is encoded into:

- exponent bucket (coarse scale)
- mantissa step (detail inside that scale)

You can tune:

- total bits: `8`, `16`, `32`
- exponent bits (or auto-pick from max expected value)
- optional `min/max` range for tighter quantization in known domains

## Install

```bash
go get numfmt
```

## Quick start

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

## Real-world style examples

```go
// requests/min in a service dashboard
reqCodec := numfmt.MustNew(numfmt.Bits16, 2, 1000)
reqCode := reqCodec.Encode(1375)
reqApprox := reqCodec.Decode(reqCode)

// duration in seconds (for spans up to ~4 weeks)
durCodec := numfmt.MustNewWithRange(numfmt.Bits16, 2, 1000, 1, 2_419_200)
durCode := durCodec.Encode(30_780) // ~8h 33m

durApprox := durCodec.Decode(durCode)

// filesize in bytes (range-constrained to your domain)
sizeCodec := numfmt.MustNewWithRange(numfmt.Bits16, 3, 1000, 1, 1_000_000_000_000)
sizeCode := sizeCodec.Encode(136_920_000)
sizeApprox := sizeCodec.Decode(sizeCode)

_, _, _ = reqApprox, durApprox, sizeApprox
```

## Auto exponent sizing

If you know the max value you care about, you can pick exponent bits automatically:

```go
expBits, err := numfmt.RecommendedExpBits(numfmt.Bits16, 1000, 100_000_000)
if err != nil {
	panic(err)
}

codec := numfmt.MustNew(numfmt.Bits16, expBits, 1000)
_ = codec
```

## Binary payload helper

You can store a whole numeric slice in a compact binary blob with a self-describing header.

Header includes:

- total bits
- exponent bits
- base
- range min/max (if enabled)
- value count

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

_, _ = restoredCodec, restoredValues
```

## Generate the quantization guide

```bash
go test -run TestGenerateQuantizationGuide
```

This generates `docs/quantization_guide.md` with concrete examples, ranges, encoded values, decoded values, and observed error.
