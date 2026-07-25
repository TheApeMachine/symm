package manifold

import (
	"encoding/binary"
	"math"
	"time"
)

const (
	binaryMagic0 = 'S'
	binaryMagic1 = 'M'
	binaryMagic2 = 'F'
	binaryMagic3 = '1'

	BinaryKindRho       uint8 = 1
	BinaryKindPsi       uint8 = 2
	BinaryKindGuidanceX uint8 = 3
	BinaryKindGuidanceZ uint8 = 4
	BinaryKindDisplay   uint8 = 5
)

/*
binaryKindKey maps a lattice kind onto the hub retain / frontend drawer key.
*/
func binaryKindKey(kind uint8) string {
	switch kind {
	case BinaryKindRho:
		return "manifold_rho"
	case BinaryKindPsi:
		return "manifold_psi"
	case BinaryKindGuidanceX:
		return "manifold_guidance_x"
	case BinaryKindGuidanceZ:
		return "manifold_guidance_z"
	case BinaryKindDisplay:
		return "manifold_display"
	default:
		return ""
	}
}

/*
BinaryCacheKey reports the retain key when payload is a manifold lattice frame.
*/
func BinaryCacheKey(payload []byte) (string, bool) {
	if len(payload) < 24 {
		return "", false
	}

	if payload[0] != binaryMagic0 || payload[1] != binaryMagic1 ||
		payload[2] != binaryMagic2 || payload[3] != binaryMagic3 {
		return "", false
	}

	key := binaryKindKey(payload[4])

	return key, key != ""
}

/*
EncodeLattice packs one X–Z lattice as little-endian uint16 samples scaled into
[min,max]. 64×64 is 8KiB payload — under the prior JSON decimal lattice size.
*/
func EncodeLattice(
	kind uint8, symbol string, at time.Time, grid [][]float64,
) ([]byte, bool) {
	if binaryKindKey(kind) == "" || symbol == "" || len(symbol) > 255 {
		return nil, false
	}

	height := len(grid)

	if height == 0 {
		return nil, false
	}

	width := len(grid[0])

	if width == 0 {
		return nil, false
	}

	for _, row := range grid {
		if len(row) != width {
			return nil, false
		}
	}

	minimum, maximum := latticeRange(grid)
	header := 4 + 1 + 2 + 2 + 4 + 4 + 8 + 1 + len(symbol)
	payload := make([]byte, header+width*height*2)
	payload[0] = binaryMagic0
	payload[1] = binaryMagic1
	payload[2] = binaryMagic2
	payload[3] = binaryMagic3
	payload[4] = kind
	binary.LittleEndian.PutUint16(payload[5:7], uint16(width))
	binary.LittleEndian.PutUint16(payload[7:9], uint16(height))
	binary.LittleEndian.PutUint32(payload[9:13], math.Float32bits(float32(minimum)))
	binary.LittleEndian.PutUint32(payload[13:17], math.Float32bits(float32(maximum)))
	binary.LittleEndian.PutUint64(payload[17:25], uint64(at.UnixNano()))
	payload[25] = byte(len(symbol))
	copy(payload[26:26+len(symbol)], symbol)

	span := maximum - minimum
	offset := 26 + len(symbol)

	for row := range height {
		for column := range width {
			sample := grid[row][column]
			coded := uint16(0)

			if span > 0 && !math.IsNaN(sample) && !math.IsInf(sample, 0) {
				unit := (sample - minimum) / span

				if unit < 0 {
					unit = 0
				}

				if unit > 1 {
					unit = 1
				}

				coded = uint16(unit*65535 + 0.5)
			}

			binary.LittleEndian.PutUint16(payload[offset:offset+2], coded)
			offset += 2
		}
	}

	return payload, true
}

/*
EncodeDisplay packs one backend-composited RGBA8 texture. One frame replaces
four scalar lattice messages; the client blits instead of re-shading planes.
*/
func EncodeDisplay(
	symbol string, at time.Time, width, height int, rgba []byte,
) ([]byte, bool) {
	if symbol == "" || len(symbol) > 255 || width <= 0 || height <= 0 {
		return nil, false
	}

	if len(rgba) != width*height*4 {
		return nil, false
	}

	header := 4 + 1 + 2 + 2 + 4 + 4 + 8 + 1 + len(symbol)
	payload := make([]byte, header+len(rgba))
	payload[0] = binaryMagic0
	payload[1] = binaryMagic1
	payload[2] = binaryMagic2
	payload[3] = binaryMagic3
	payload[4] = BinaryKindDisplay
	binary.LittleEndian.PutUint16(payload[5:7], uint16(width))
	binary.LittleEndian.PutUint16(payload[7:9], uint16(height))
	binary.LittleEndian.PutUint32(payload[9:13], math.Float32bits(0))
	binary.LittleEndian.PutUint32(payload[13:17], math.Float32bits(1))
	binary.LittleEndian.PutUint64(payload[17:25], uint64(at.UnixNano()))
	payload[25] = byte(len(symbol))
	copy(payload[26:26+len(symbol)], symbol)
	copy(payload[26+len(symbol):], rgba)

	return payload, true
}

func latticeRange(grid [][]float64) (float64, float64) {
	minimum := math.Inf(1)
	maximum := math.Inf(-1)

	for _, row := range grid {
		for _, sample := range row {
			if math.IsNaN(sample) || math.IsInf(sample, 0) {
				continue
			}

			if sample < minimum {
				minimum = sample
			}

			if sample > maximum {
				maximum = sample
			}
		}
	}

	if math.IsInf(minimum, 0) || math.IsInf(maximum, 0) {
		return 0, 0
	}

	return minimum, maximum
}
