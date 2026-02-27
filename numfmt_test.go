package numfmt

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

	profiles := []profile{
		{name: "Simplistic-8bit", codec: MustNew(Bits8, 3, 1000), sampleMin: 1, sampleMax: 1_000_000, description: "Tiny payloads and rough telemetry."},
		{name: "Balanced-16bit", codec: MustNew(Bits16, 3, 1000), sampleMin: 1, sampleMax: 1_000_000_000, description: "Default profile for broad SI ranges."},
		{name: "Precision-32bit", codec: MustNew(Bits32, 6, 1000), sampleMin: 1, sampleMax: 1_000_000_000_000, description: "Higher detail while still compact."},
		{name: "OptionsGalore-Range16", codec: MustNewWithRange(Bits16, 3, 1000, 10_000, 200_000), sampleMin: 10_000, sampleMax: 200_000, description: "Bounded domain for much lower quantization in a working interval."},
		{name: "OptionsGalore-Range32", codec: MustNewWithRange(Bits32, 6, 1000, 100, 10_000_000), sampleMin: 100, sampleMax: 10_000_000, description: "Wide option set with custom bounded operating window."},
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
		mantBits := p.codec.mantBits()
		mantLevels := p.codec.mantMask()
		stepFactor := 1.0
		ballparkRel := 0.0
		if mantLevels > 1 {
			stepFactor = math.Pow(p.codec.Base, 1.0/float64(mantLevels-1))
			ballparkRel = math.Sqrt(stepFactor) - 1
		}

		b.WriteString(fmt.Sprintf("## %s\n\n", p.name))
		b.WriteString(fmt.Sprintf("%s\n\n", p.description))
		b.WriteString(fmt.Sprintf("- total bits: `%d`\n", p.codec.TotalBits))
		b.WriteString(fmt.Sprintf("- exponent bits: `%d`\n", p.codec.ExpBits))
		b.WriteString(fmt.Sprintf("- mantissa bits: `%d`\n", mantBits))
		b.WriteString(fmt.Sprintf("- mantissa levels (non-zero): `%d`\n", mantLevels))
		b.WriteString(fmt.Sprintf("- base: `%s`\n", fmtNoExp(p.codec.Base)))
		if p.codec.RangeEnabled() {
			b.WriteString(fmt.Sprintf("- range: `[%s, %s]` (range-constrained mode)\n", fmtNoExp(p.codec.RangeMin), fmtNoExp(p.codec.RangeMax)))
		} else {
			b.WriteString("- range: unbounded SI-style mode\n")
		}
		b.WriteString(fmt.Sprintf("- typical step size between nearby values: `~%.2f%%`\n", 100*(stepFactor-1)))
		b.WriteString(fmt.Sprintf("- max rounding error from one encode/decode step: `~%.2f%%`\n\n", 100*ballparkRel))

		b.WriteString("Exponent levels 0-3 (rough scale): `1k, 1M, 1B, 1T`\n\n")

		b.WriteString("Application examples:\n\n")
		type example struct {
			label string
			value float64
		}

		examples := []example{
			{label: "requests/min", value: 1_375},
			{label: "miles distance", value: 242.7},
			{label: "duration (sec)", value: 5_820},
			{label: "pounds", value: 186.4},
			{label: "filesize (bytes)", value: 136_920_000},
		}
		if p.codec.RangeEnabled() {
			spanLog := math.Log(p.sampleMax / p.sampleMin)
			examples = []example{
				{label: "requests/min", value: p.sampleMin * math.Exp(0.10*spanLog)},
				{label: "miles distance", value: p.sampleMin * math.Exp(0.25*spanLog)},
				{label: "duration (sec)", value: p.sampleMin * math.Exp(0.45*spanLog)},
				{label: "pounds", value: p.sampleMin * math.Exp(0.65*spanLog)},
				{label: "filesize (bytes)", value: p.sampleMin * math.Exp(0.90*spanLog)},
			}
		}

		checkpointRows := make([][]string, 0, len(examples))
		hasError := false
		for _, ex := range examples {
			code := p.codec.Encode(ex.value)
			decoded := p.codec.Decode(code)
			errPct := 100 * math.Abs(decoded-ex.value) / ex.value
			errCell := ""
			errFmt := fmt.Sprintf("%.2f%%", errPct)
			if errFmt != "0.00%" {
				errCell = errFmt
				hasError = true
			}
			checkpointRows = append(checkpointRows, []string{
				ex.label,
				humanSI(ex.value),
				fmt.Sprintf("%d", code),
				humanSI(decoded),
				errCell,
			})
		}
		if hasError {
			writeAlignedTable(&b, []string{"example", "input", "code", "decoded", "error"}, checkpointRows)
		} else {
			rows := make([][]string, 0, len(checkpointRows))
			for _, row := range checkpointRows {
				rows = append(rows, row[:4])
			}
			writeAlignedTable(&b, []string{"example", "input", "code", "decoded"}, rows)
		}
		b.WriteString("\n")

		b.WriteString("Quantization error by range (log-spaced bins):\n\n")
		const bins = 12
		type binStat struct {
			n   int
			sum float64
			max float64
		}
		stats := make([]binStat, bins)
		const samples = 4800
		for i := 0; i < samples; i++ {
			tv := (float64(i) + 0.5) / float64(samples)
			v := p.sampleMin * math.Exp(tv*math.Log(p.sampleMax/p.sampleMin))
			code := p.codec.Encode(v)
			decoded := p.codec.Decode(code)
			err := math.Abs(decoded-v) / v
			idx := int(tv * bins)
			if idx >= bins {
				idx = bins - 1
			}
			stats[idx].n++
			stats[idx].sum += err
			if err > stats[idx].max {
				stats[idx].max = err
			}
		}

		rangeRows := make([][]string, 0, bins)
		avgMinPct := math.MaxFloat64
		avgMaxPct := 0.0
		worstMinPct := math.MaxFloat64
		worstMaxPct := 0.0

		for i := 0; i < bins; i++ {
			loT := float64(i) / float64(bins)
			hiT := float64(i+1) / float64(bins)
			lo := p.sampleMin * math.Exp(loT*math.Log(p.sampleMax/p.sampleMin))
			hi := p.sampleMin * math.Exp(hiT*math.Log(p.sampleMax/p.sampleMin))

			avgPct := 0.0
			if stats[i].n > 0 {
				avgPct = 100 * (stats[i].sum / float64(stats[i].n))
			}
			worstPct := 100 * stats[i].max

			if avgPct < avgMinPct {
				avgMinPct = avgPct
			}
			if avgPct > avgMaxPct {
				avgMaxPct = avgPct
			}
			if worstPct < worstMinPct {
				worstMinPct = worstPct
			}
			if worstPct > worstMaxPct {
				worstMaxPct = worstPct
			}

			rangeRows = append(rangeRows, []string{
				fmt.Sprintf("[%s, %s)", humanSI(lo), humanSI(hi)),
				fmt.Sprintf("%.2f%%", avgPct),
				fmt.Sprintf("%.2f%%", worstPct),
			})
		}

		avgSpread := avgMaxPct - avgMinPct
		worstSpread := worstMaxPct - worstMinPct
		const uniformEps = 0.0005

		if avgSpread <= uniformEps && worstSpread <= uniformEps {
			b.WriteString("Errors are effectively uniform across this range.\n\n")
			summaryRows := [][]string{
				{"avg error", fmt.Sprintf("%.2f%%", avgMaxPct)},
				{"worst error", fmt.Sprintf("%.2f%%", worstMaxPct)},
			}
			writeAlignedTable(&b, []string{"metric", "value"}, summaryRows)
		} else {
			writeAlignedTable(&b, []string{"range", "avg error", "worst error"}, rangeRows)
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
	for _, want := range []string{"Quantization Guide", "Quantization error by range", "Application examples", "worst-case", "requests/min"} {
		if !strings.Contains(s, want) {
			t.Fatalf("generated guide missing %q", want)
		}
	}
}
