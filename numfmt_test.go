package numfmt

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	durafmt "github.com/hako/durafmt"
	"testing"
)

func fmtNoExp(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Sprintf("%v", v)
	}
	decimals := 2
	s := strconv.FormatFloat(v, 'f', decimals, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	if s == "" || s == "-0" {
		return "0"
	}
	return s
}

func humanSI(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Sprintf("%v", v)
	}
	if v == 0 {
		return "0"
	}
	suffixes := []string{"", "k", "M", "B", "T", "P", "E"}
	abs := math.Abs(v)
	idx := 0
	for abs >= 1000 && idx < len(suffixes)-1 {
		abs /= 1000
		idx++
	}
	if v < 0 {
		abs = -abs
	}
	return fmt.Sprintf("%s%s", fmtNoExp(abs), suffixes[idx])
}

func humanDurationSeconds(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Sprintf("%v", v)
	}
	if v <= 0 {
		return "0s"
	}
	d := time.Duration(math.Round(v)) * time.Second
	long := durafmt.Parse(d).LimitFirstN(2).String()
	repl := strings.NewReplacer(
		" years", "y", " year", "y",
		" months", "mo", " month", "mo",
		" weeks", "w", " week", "w",
		" days", "d", " day", "d",
		" hours", "h", " hour", "h",
		" minutes", "m", " minute", "m",
		" seconds", "s", " second", "s",
	)
	short := repl.Replace(long)
	short = strings.ReplaceAll(short, ",", "")
	return strings.TrimSpace(short)
}

func formatExampleValue(label string, v float64) string {
	if strings.Contains(label, "duration") {
		return humanDurationSeconds(v)
	}
	return humanSI(v)
}

func humanBytes(v float64) string {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Sprintf("%v", v)
	}
	if v < 0 {
		v = 0
	}
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	idx := 0
	for v >= 1024 && idx < len(units)-1 {
		v /= 1024
		idx++
	}
	return fmt.Sprintf("%s%s", fmtNoExp(v), units[idx])
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func writeAlignedTable(b *strings.Builder, headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, row := range rows {
		for i := range headers {
			if i < len(row) && len(row[i]) > widths[i] {
				widths[i] = len(row[i])
			}
		}
	}

	b.WriteString("| ")
	for i, h := range headers {
		if i > 0 {
			b.WriteString(" | ")
		}
		b.WriteString(padRight(h, widths[i]))
	}
	b.WriteString(" |\n")

	b.WriteString("|-")
	for i := range headers {
		if i > 0 {
			b.WriteString("-|-")
		}
		b.WriteString(strings.Repeat("-", widths[i]))
	}
	b.WriteString("-|\n")

	for _, row := range rows {
		b.WriteString("| ")
		for i := range headers {
			if i > 0 {
				b.WriteString(" | ")
			}
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			b.WriteString(padRight(cell, widths[i]))
		}
		b.WriteString(" |\n")
	}
}

func errorBar(value, max float64, width int) string {
	if max <= 0 || value <= 0 {
		return ""
	}
	n := int(math.Round((value / max) * float64(width)))
	if n < 1 {
		n = 1
	}
	if n > width {
		n = width
	}
	return strings.Repeat("#", n)
}

func TestNewValidation(t *testing.T) {
	if _, err := New(12, 3, 1000); err == nil {
		t.Fatal("expected error for unsupported totalBits")
	}
	if _, err := New(Bits16, 0, 1000); err == nil {
		t.Fatal("expected error for expBits=0")
	}
	if _, err := New(Bits16, Bits16, 1000); err == nil {
		t.Fatal("expected error for expBits=totalBits")
	}
	if _, err := New(Bits16, 3, 1); err == nil {
		t.Fatal("expected error for base=1")
	}
	if _, err := NewWithRange(Bits16, 3, 1000, 0, 1000); err == nil {
		t.Fatal("expected error for minValue=0")
	}
	if _, err := NewWithRange(Bits16, 3, 1000, 100, 100); err == nil {
		t.Fatal("expected error for maxValue=minValue")
	}
}

func TestZero_DefaultCodec(t *testing.T) {
	if got := Encode(0); got != 0 {
		t.Fatalf("Encode(0)=%d want 0", got)
	}
	if got := Decode(0); got != 0 {
		t.Fatalf("Decode(0)=%v want 0", got)
	}
}

func TestRoundTripRelativeError_DefaultCodec(t *testing.T) {
	values := []float64{1, 12.345, 999.9, 1000, 1234.56, 999_999, 1_234_567, 75_000_000_000, 123_456_789_012_345}
	for _, v := range values {
		code := Encode(v)
		got := Decode(code)
		if got <= 0 {
			t.Fatalf("Decode(Encode(%v))=%v", v, got)
		}
		relErr := math.Abs(got-v) / v
		if relErr > 1e-3 {
			t.Fatalf("roundtrip relErr too high for %v: got=%v code=%d relErr=%g", v, got, code, relErr)
		}
	}
}

func TestMonotonic_DefaultCodec(t *testing.T) {
	values := []float64{0, 1, 2, 10, 999.9, 1000, 1001, 10_000, 1_000_000, 1_000_000_000, 100_000_000_000_000}
	prev := uint16(0)
	for i, v := range values {
		code := Encode(v)
		if i > 0 && code < prev {
			t.Fatalf("non-monotonic: v=%v code=%d prev=%d", v, code, prev)
		}
		prev = code
	}
}

func TestSaturates_DefaultCodec(t *testing.T) {
	maxCode := uint16(DefaultCodec.maxCode())
	if got := Encode(math.Inf(1)); got != maxCode {
		t.Fatalf("Encode(+Inf)=%d want %d", got, maxCode)
	}
	if got := Encode(1e300); got != maxCode {
		t.Fatalf("Encode(1e300)=%d want %d", got, maxCode)
	}
}

func TestFormats8_16_32(t *testing.T) {
	tests := []struct {
		totalBits uint8
		expBits   uint8
		maxRelErr float64
	}{
		{totalBits: Bits8, expBits: 3, maxRelErr: 0.20},
		{totalBits: Bits16, expBits: 3, maxRelErr: 1e-3},
		{totalBits: Bits32, expBits: 6, maxRelErr: 2e-6},
	}

	values := []float64{1, 3.14159, 42, 999.9, 1000, 10_001, 1_234_567, 75_000_000_000}

	for _, tc := range tests {
		c := MustNew(tc.totalBits, tc.expBits, 1000)
		prev := uint64(0)
		for i, v := range values {
			code := c.Encode(v)
			if i > 0 && code < prev {
				t.Fatalf("non-monotonic for %d-bit codec: v=%v code=%d prev=%d", tc.totalBits, v, code, prev)
			}
			prev = code

			got := c.Decode(code)
			relErr := math.Abs(got-v) / v
			if relErr > tc.maxRelErr {
				t.Fatalf("%d-bit relErr too high for %v: got=%v code=%d relErr=%g", tc.totalBits, v, got, code, relErr)
			}
		}
	}
}

func TestRangeCodec_ReducesQuantizationOnBoundedDomain(t *testing.T) {
	unbounded := MustNew(Bits16, 3, 1000)
	ranged := MustNewWithRange(Bits16, 3, 1000, 10_000, 200_000)
	if !ranged.RangeEnabled() {
		t.Fatal("expected ranged codec")
	}

	const samples = 300
	var unboundedErr float64
	var rangedErr float64

	for i := 0; i < samples; i++ {
		t := float64(i) / float64(samples-1)
		v := 10_000 * math.Exp(t*math.Log(200_000.0/10_000.0))

		u := unbounded.Decode(unbounded.Encode(v))
		r := ranged.Decode(ranged.Encode(v))
		unboundedErr += math.Abs(u-v) / v
		rangedErr += math.Abs(r-v) / v
	}

	if rangedErr >= unboundedErr {
		t.Fatalf("expected ranged codec to reduce quantization error: unbounded=%g ranged=%g", unboundedErr, rangedErr)
	}
}

func TestTypedEncodeDecodeHelpers(t *testing.T) {
	c8 := MustNew(Bits8, 3, 1000)
	if got := c8.Decode8(c8.Encode8(12345)); got <= 0 {
		t.Fatalf("Decode8(Encode8())=%v", got)
	}

	c16 := MustNew(Bits16, 3, 1000)
	if got := c16.Decode16(c16.Encode16(12345)); got <= 0 {
		t.Fatalf("Decode16(Encode16())=%v", got)
	}

	c32 := MustNew(Bits32, 6, 1000)
	if got := c32.Decode32(c32.Encode32(12345)); got <= 0 {
		t.Fatalf("Decode32(Encode32())=%v", got)
	}
}

func TestGenerateQuantizationGuide(t *testing.T) {
	type profile struct {
		name        string
		codec       Codec
		sampleMin   float64
		sampleMax   float64
		description string
	}

	autoExp := func(totalBits uint8, maxValue float64) uint8 {
		b, err := RecommendedExpBits(totalBits, 1000, maxValue)
		if err != nil {
			t.Fatalf("recommended exp bits: %v", err)
		}
		return b
	}

	profiles := []profile{
		{name: "Simplistic-8bit", codec: MustNew(Bits8, autoExp(Bits8, 1_000_000), 1000), sampleMin: 1, sampleMax: 1_000_000, description: "Tiny payloads and rough telemetry."},
		{name: "Balanced-16bit", codec: MustNew(Bits16, autoExp(Bits16, 1_000_000_000), 1000), sampleMin: 1, sampleMax: 1_000_000_000, description: "Default profile for broad SI ranges."},
		{name: "Precision-32bit", codec: MustNew(Bits32, autoExp(Bits32, 1_000_000_000_000), 1000), sampleMin: 1, sampleMax: 1_000_000_000_000, description: "Higher detail while still compact."},
	}
	var b strings.Builder
	b.WriteString("# Quantization Guide\n\n")
	b.WriteString("This file is generated by `go test` (TestGenerateQuantizationGuide).\n\n")
	b.WriteString("## How to Read This\n\n")
	b.WriteString("- Exponent bucket: chooses coarse scale by powers of `base`.\n")
	b.WriteString("- Mantissa levels: fine-grained steps inside each exponent bucket.\n")
	b.WriteString("- Range mode: uses all non-zero codes across a chosen `[min,max]` interval.\n")
	b.WriteString("- Range error table: log-binned average and worst-case relative error.\n\n")

	for _, p := range profiles {

		b.WriteString(fmt.Sprintf("## %s\n\n", p.name))
		b.WriteString(fmt.Sprintf("%s\n\n", p.description))
		const sampleValues = 100_000_000
		bytes64 := float64(sampleValues * 8)
		bytesFmt := float64(sampleValues) * float64(p.codec.TotalBits) / 8
		savedBytes := bytes64 - bytesFmt
		savedPct := 100 * savedBytes / bytes64
		b.WriteString(fmt.Sprintf("- space 100m values: `%s` int64: `%s` (%.2f%% smaller)\n\n", humanBytes(bytesFmt), humanBytes(bytes64), savedPct))

		b.WriteString("Application examples:\n\n")
		type example struct {
			label string
			value float64
			min   float64
			max   float64
			cap   float64
		}

		durationSeconds := float64(30_780) // 8h 33m
		examples := []example{
			{label: "requests/min", value: 1_375, min: 0, max: 100_000_000, cap: 1_000_000_000},
			{label: "miles distance", value: 242.7, min: 0, max: 100_000, cap: 1_000_000},
			{label: "duration (unix)", value: durationSeconds, min: 0, max: 2_419_200, cap: 31_536_000}, // 0 to 4 weeks
			{label: "pounds", value: 186.4, min: 0, max: 1_000, cap: 10_000},
			{label: "filesize (bytes)", value: 136_920_000, min: 0, max: 1_000_000_000_000_000, cap: 100_000_000_000_000_000},
		}
		if p.codec.RangeEnabled() && strings.Contains(p.name, "Range16") {
			examples = []example{
				{label: "requests/min", value: 1_375, min: 0, max: 1_000_000, cap: 100_000_000},
				{label: "miles distance", value: 242.7, min: 0, max: 100_000, cap: 1_000_000},
				{label: "duration (unix)", value: durationSeconds, min: 0, max: 2_419_200, cap: 31_536_000},
				{label: "pounds", value: 186.4, min: 0, max: 1_000, cap: 10_000},
				{label: "filesize (bytes)", value: 136_920_000, min: 0, max: 100_000_000_000, cap: 1_000_000_000_000},
			}
		}

		checkpointRows := make([][]string, 0, len(examples))
		hasError := false

		buildCodec := func(minV, maxV float64) (Codec, uint8, error) {
			expBits, err := RecommendedExpBits(p.codec.TotalBits, p.codec.Base, maxV)
			if err != nil {
				return Codec{}, 0, err
			}
			if p.codec.RangeEnabled() {
				codecMin := minV
				if codecMin <= 0 {
					codecMin = math.Max(maxV*1e-9, 1e-9)
				}
				c, err := NewWithRange(p.codec.TotalBits, expBits, p.codec.Base, codecMin, maxV)
				return c, expBits, err
			}
			c, err := New(p.codec.TotalBits, expBits, p.codec.Base)
			return c, expBits, err
		}

		for _, ex := range examples {
			tunedMin := ex.min
			tunedMax := ex.max

			var codecForRow Codec
			var expBits uint8
			var code uint64
			var decoded float64
			var errPct float64
			var err error

			for pass := 0; pass < 3; pass++ {
				codecForRow, expBits, err = buildCodec(tunedMin, tunedMax)
				if err != nil {
					t.Fatalf("row codec: %v", err)
				}
				code = codecForRow.Encode(ex.value)
				decoded = codecForRow.Decode(code)
				errPct = 100 * math.Abs(decoded-ex.value) / ex.value

				// If error is high, tighten range to improve 8-bit readability.
				if errPct > 2.00 {
					nextMax := tunedMax / 10
					if nextMax > ex.value*2 && nextMax >= ex.max/100 {
						tunedMax = nextMax
						continue
					}
				}

				// If error is already tiny, broaden range for a more realistic tradeoff.
				if errPct < 0.05 {
					nextMax := tunedMax * 10
					capMax := ex.cap
					if capMax <= ex.max {
						capMax = ex.max * 10
					}
					if capMax < ex.value*2 {
						capMax = ex.value * 2
					}
					if nextMax <= tunedMax || nextMax > capMax {
						break
					}
					tunedMax = nextMax
					if tunedMin > 0 {
						tunedMin /= 10
					}
					continue
				}
				break
			}

			errCell := ""
			errFmt := fmt.Sprintf("%.2f%%", errPct)
			if errFmt != "0.00%" {
				errCell = errFmt
				hasError = true
			}

			checkpointRows = append(checkpointRows, []string{
				ex.label,
				fmt.Sprintf("%s-%s", formatExampleValue(ex.label, tunedMin), formatExampleValue(ex.label, tunedMax)),
				fmt.Sprintf("%d", expBits),
				formatExampleValue(ex.label, ex.value),
				fmt.Sprintf("%d", code),
				formatExampleValue(ex.label, decoded),
				errCell,
			})
		}

		if hasError {
			writeAlignedTable(&b, []string{"example", "min/max", "exp", "input", "code", "decoded", "error"}, checkpointRows)
		} else {
			rows := make([][]string, 0, len(checkpointRows))
			for _, row := range checkpointRows {
				rows = append(rows, row[:6])
			}
			writeAlignedTable(&b, []string{"example", "min/max", "exp", "input", "code", "decoded"}, rows)
		}
		b.WriteString("\n")
	}

	outPath := filepath.Join("docs", "quantization_guide.md")
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(outPath, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write guide: %v", err)
	}

	content, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read guide: %v", err)
	}
	s := string(content)
	for _, want := range []string{"Quantization Guide", "Application examples", "requests/min"} {
		if !strings.Contains(s, want) {
			t.Fatalf("generated guide missing %q", want)
		}
	}
}

func TestRecommendedExpBits(t *testing.T) {
	cases := []struct {
		name      string
		totalBits uint8
		base      float64
		maxValue  float64
		want      uint8
	}{
		{name: "8bit 1M", totalBits: Bits8, base: 1000, maxValue: 1_000_000, want: 2},
		{name: "16bit 1B", totalBits: Bits16, base: 1000, maxValue: 1_000_000_000, want: 2},
		{name: "32bit 1T", totalBits: Bits32, base: 1000, maxValue: 1_000_000_000_000, want: 3},
		{name: "small max", totalBits: Bits16, base: 1000, maxValue: 10, want: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RecommendedExpBits(tc.totalBits, tc.base, tc.maxValue)
			if err != nil {
				t.Fatalf("RecommendedExpBits() err = %v", err)
			}
			if got != tc.want {
				t.Fatalf("RecommendedExpBits() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestRecommendedExpBitsValidation(t *testing.T) {
	if _, err := RecommendedExpBits(12, 1000, 1000); err == nil {
		t.Fatal("expected error for unsupported totalBits")
	}
	if _, err := RecommendedExpBits(Bits16, 1, 1000); err == nil {
		t.Fatal("expected error for invalid base")
	}
	if _, err := RecommendedExpBits(Bits16, 1000, 0); err == nil {
		t.Fatal("expected error for invalid maxValue")
	}
}

func TestBinaryRoundTrip_Unbounded(t *testing.T) {
	codec := MustNew(Bits16, 2, 1000)
	in := []float64{0, 1, 10, 999.9, 1_234, 88_000, 7_500_000}

	blob, err := EncodeValuesBinary(codec, in)
	if err != nil {
		t.Fatalf("EncodeValuesBinary() err = %v", err)
	}

	h, err := DecodeBinaryHeader(blob)
	if err != nil {
		t.Fatalf("DecodeBinaryHeader() err = %v", err)
	}
	if h.ExpBits != codec.ExpBits || h.TotalBits != codec.TotalBits {
		t.Fatalf("header bits mismatch: got total=%d exp=%d", h.TotalBits, h.ExpBits)
	}
	if h.UseRange {
		t.Fatal("header unexpectedly marked as ranged")
	}

	decodedCodec, out, err := DecodeValuesBinary(blob)
	if err != nil {
		t.Fatalf("DecodeValuesBinary() err = %v", err)
	}
	if decodedCodec.TotalBits != codec.TotalBits || decodedCodec.ExpBits != codec.ExpBits || decodedCodec.Base != codec.Base {
		t.Fatalf("decoded codec mismatch: got %+v want %+v", decodedCodec, codec)
	}
	if len(out) != len(in) {
		t.Fatalf("decoded length mismatch: got %d want %d", len(out), len(in))
	}
	for i := range in {
		want := codec.Decode(codec.Encode(in[i]))
		if out[i] != want {
			t.Fatalf("decoded[%d]=%v want %v", i, out[i], want)
		}
	}
}

func TestBinaryRoundTrip_Ranged(t *testing.T) {
	codec := MustNewWithRange(Bits32, 3, 1000, 1, 2_419_200)
	in := []float64{1, 60, 3_600, 30_780, 86_400, 2_000_000}

	blob, err := EncodeValuesBinary(codec, in)
	if err != nil {
		t.Fatalf("EncodeValuesBinary() err = %v", err)
	}

	h, err := DecodeBinaryHeader(blob)
	if err != nil {
		t.Fatalf("DecodeBinaryHeader() err = %v", err)
	}
	if !h.UseRange {
		t.Fatal("expected range flag in header")
	}
	if h.RangeMin != codec.RangeMin || h.RangeMax != codec.RangeMax {
		t.Fatalf("header range mismatch: got [%v,%v] want [%v,%v]", h.RangeMin, h.RangeMax, codec.RangeMin, codec.RangeMax)
	}

	decodedCodec, out, err := DecodeValuesBinary(blob)
	if err != nil {
		t.Fatalf("DecodeValuesBinary() err = %v", err)
	}
	if !decodedCodec.RangeEnabled() {
		t.Fatal("decoded codec should be ranged")
	}
	if decodedCodec.RangeMin != codec.RangeMin || decodedCodec.RangeMax != codec.RangeMax {
		t.Fatalf("decoded codec range mismatch: got [%v,%v] want [%v,%v]", decodedCodec.RangeMin, decodedCodec.RangeMax, codec.RangeMin, codec.RangeMax)
	}
	for i := range in {
		want := codec.Decode(codec.Encode(in[i]))
		if out[i] != want {
			t.Fatalf("decoded[%d]=%v want %v", i, out[i], want)
		}
	}
}

func TestBinaryDecodeValidation(t *testing.T) {
	codec := MustNew(Bits8, 2, 1000)
	blob, err := EncodeValuesBinary(codec, []float64{1, 2, 3})
	if err != nil {
		t.Fatalf("EncodeValuesBinary() err = %v", err)
	}

	badMagic := append([]byte(nil), blob...)
	badMagic[0] = 'X'
	if _, _, err := DecodeValuesBinary(badMagic); err == nil {
		t.Fatal("expected bad magic error")
	}

	badVersion := append([]byte(nil), blob...)
	badVersion[4] = 9
	if _, _, err := DecodeValuesBinary(badVersion); err == nil {
		t.Fatal("expected bad version error")
	}

	truncated := blob[:len(blob)-1]
	if _, _, err := DecodeValuesBinary(truncated); err == nil {
		t.Fatal("expected truncated payload error")
	}
}
