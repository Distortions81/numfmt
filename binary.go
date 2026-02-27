package numfmt

import (
	"encoding/binary"
	"fmt"
	"math"
)

const (
	binaryMagic   = "NMF1"
	binaryVersion = uint8(1)
	headerSize    = 40
)

const (
	headerFlagRange = 1 << 0
)

// BinaryHeader stores codec metadata persisted in a binary payload header.
type BinaryHeader struct {
	Version   uint8
	TotalBits uint8
	ExpBits   uint8
	Base      float64
	RangeMin  float64
	RangeMax  float64
	UseRange  bool
	Count     uint64
}

// EncodeValuesBinary encodes values into a compact binary payload.
//
// Format:
// - 4 bytes magic: "NMF1"
// - 1 byte version
// - 1 byte total bits
// - 1 byte exponent bits
// - 1 byte flags (bit0: range mode)
// - 8 bytes base (float64 little-endian bits)
// - 8 bytes min (float64 little-endian bits)
// - 8 bytes max (float64 little-endian bits)
// - 8 bytes count (uint64 little-endian)
// - bit-packed codes (little-endian bit order, width=total bits)
func EncodeValuesBinary(c Codec, values []float64) ([]byte, error) {
	if _, err := New(c.TotalBits, c.ExpBits, c.Base); err != nil {
		return nil, err
	}
	if c.useRange {
		if _, err := c.WithRange(c.RangeMin, c.RangeMax); err != nil {
			return nil, err
		}
	}

	codes := make([]uint64, len(values))
	for i, v := range values {
		codes[i] = c.Encode(v)
	}
	payload, err := packCodes(c.TotalBits, codes)
	if err != nil {
		return nil, err
	}

	out := make([]byte, headerSize+len(payload))
	copy(out[0:4], []byte(binaryMagic))
	out[4] = binaryVersion
	out[5] = c.TotalBits
	out[6] = c.ExpBits
	if c.useRange {
		out[7] = headerFlagRange
	}
	binary.LittleEndian.PutUint64(out[8:16], math.Float64bits(c.Base))
	binary.LittleEndian.PutUint64(out[16:24], math.Float64bits(c.RangeMin))
	binary.LittleEndian.PutUint64(out[24:32], math.Float64bits(c.RangeMax))
	binary.LittleEndian.PutUint64(out[32:40], uint64(len(values)))
	copy(out[headerSize:], payload)
	return out, nil
}

// DecodeValuesBinary decodes a binary payload produced by EncodeValuesBinary.
func DecodeValuesBinary(data []byte) (Codec, []float64, error) {
	h, err := DecodeBinaryHeader(data)
	if err != nil {
		return Codec{}, nil, err
	}

	codec, err := New(h.TotalBits, h.ExpBits, h.Base)
	if err != nil {
		return Codec{}, nil, err
	}
	if h.UseRange {
		codec, err = codec.WithRange(h.RangeMin, h.RangeMax)
		if err != nil {
			return Codec{}, nil, err
		}
	}

	payloadBits := h.Count * uint64(h.TotalBits)
	payloadBytes := int((payloadBits + 7) / 8)
	if len(data) != headerSize+payloadBytes {
		return Codec{}, nil, fmt.Errorf("invalid payload size: got %d, want %d", len(data), headerSize+payloadBytes)
	}

	codes, err := unpackCodes(h.TotalBits, h.Count, data[headerSize:])
	if err != nil {
		return Codec{}, nil, err
	}
	values := make([]float64, len(codes))
	for i, code := range codes {
		values[i] = codec.Decode(code)
	}
	return codec, values, nil
}

// DecodeBinaryHeader parses only the binary header.
func DecodeBinaryHeader(data []byte) (BinaryHeader, error) {
	if len(data) < headerSize {
		return BinaryHeader{}, fmt.Errorf("binary payload too small: got %d, need at least %d", len(data), headerSize)
	}
	if string(data[0:4]) != binaryMagic {
		return BinaryHeader{}, fmt.Errorf("invalid magic")
	}
	if data[4] != binaryVersion {
		return BinaryHeader{}, fmt.Errorf("unsupported version: %d", data[4])
	}

	h := BinaryHeader{
		Version:   data[4],
		TotalBits: data[5],
		ExpBits:   data[6],
		UseRange:  data[7]&headerFlagRange != 0,
		Base:      math.Float64frombits(binary.LittleEndian.Uint64(data[8:16])),
		RangeMin:  math.Float64frombits(binary.LittleEndian.Uint64(data[16:24])),
		RangeMax:  math.Float64frombits(binary.LittleEndian.Uint64(data[24:32])),
		Count:     binary.LittleEndian.Uint64(data[32:40]),
	}

	if _, err := New(h.TotalBits, h.ExpBits, h.Base); err != nil {
		return BinaryHeader{}, err
	}
	if h.UseRange {
		c := Codec{TotalBits: h.TotalBits, ExpBits: h.ExpBits, Base: h.Base}
		if _, err := c.WithRange(h.RangeMin, h.RangeMax); err != nil {
			return BinaryHeader{}, err
		}
	}
	return h, nil
}

func packCodes(totalBits uint8, codes []uint64) ([]byte, error) {
	if !isValidTotalBits(totalBits) {
		return nil, fmt.Errorf("totalBits must be one of [8,16,32], got %d", totalBits)
	}
	mask := uint64(1<<totalBits) - 1
	totalBitsLen := uint64(len(codes)) * uint64(totalBits)
	out := make([]byte, (totalBitsLen+7)/8)

	bitPos := uint64(0)
	for _, code := range codes {
		code &= mask
		for b := uint8(0); b < totalBits; b++ {
			if (code & (1 << b)) != 0 {
				byteIdx := bitPos / 8
				bitIdx := bitPos % 8
				out[byteIdx] |= byte(1 << bitIdx)
			}
			bitPos++
		}
	}
	return out, nil
}

func unpackCodes(totalBits uint8, count uint64, payload []byte) ([]uint64, error) {
	if !isValidTotalBits(totalBits) {
		return nil, fmt.Errorf("totalBits must be one of [8,16,32], got %d", totalBits)
	}
	totalBitsLen := count * uint64(totalBits)
	wantBytes := (totalBitsLen + 7) / 8
	if uint64(len(payload)) != wantBytes {
		return nil, fmt.Errorf("invalid packed code size: got %d, want %d", len(payload), wantBytes)
	}

	codes := make([]uint64, count)
	bitPos := uint64(0)
	for i := uint64(0); i < count; i++ {
		var code uint64
		for b := uint8(0); b < totalBits; b++ {
			byteIdx := bitPos / 8
			bitIdx := bitPos % 8
			if payload[byteIdx]&(1<<bitIdx) != 0 {
				code |= (1 << b)
			}
			bitPos++
		}
		codes[i] = code
	}
	return codes, nil
}
