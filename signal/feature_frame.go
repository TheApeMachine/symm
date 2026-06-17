package signal

import (
	"encoding/binary"
	"io"
	"math"
	"sort"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
)

const (
	// FeatureFrameSize is the capnp wire budget for one feature artifact frame.
	FeatureFrameSize = 32 * 1024

	// ExcitationPayloadHeader is the fixed float header on hawkes excitation payloads.
	ExcitationPayloadHeader = 4

	float64WireBytes = 8
)

/*
FeedRingCapacity returns the configured per-symbol feed ring depth.
*/
func FeedRingCapacity() int {
	capacity := viper.GetInt("signals.feed_ring_capacity")

	if capacity < 4 {
		return 64
	}

	return capacity
}

/*
EncodePayload serializes float64 samples as big-endian IEEE754 bytes.
*/
func EncodePayload(samples ...float64) []byte {
	payload := make([]byte, float64WireBytes*len(samples))

	for index, sample := range samples {
		offset := index * float64WireBytes
		binary.BigEndian.PutUint64(payload[offset:offset+float64WireBytes], math.Float64bits(sample))
	}

	return payload
}

/*
ReadFeatureArtifact marshals artifact capnp wire into buffer.
*/
func ReadFeatureArtifact(buffer []byte, artifact *datura.Artifact) (int, error) {
	if artifact == nil {
		return 0, io.EOF
	}

	wire, err := artifact.Message().Marshal()

	if err != nil {
		return 0, err
	}

	if len(wire) > len(buffer) {
		return 0, io.ErrShortBuffer
	}

	return copy(buffer, wire), nil
}

/*
MaxFeatureFloats returns the float payload capacity that still fits FeatureFrameSize
once encrypted and marshaled for the given artifact identity.
*/
func MaxFeatureFloats(origin, role, scope string, headerFloats int) int {
	if headerFloats < 0 {
		return 0
	}

	probe := func(floatCount int) int {
		if floatCount < headerFloats {
			return FeatureFrameSize + 1
		}

		samples := make([]float64, floatCount)

		for index := range samples {
			samples[index] = float64(index + 1)
		}

		artifact := datura.Acquire(origin, datura.Artifact_Type_json)

		if artifact == nil {
			return FeatureFrameSize + 1
		}

		artifact.WithRole(role)
		artifact.WithScope(scope)
		artifact.WithPayload(EncodePayload(samples...))

		wire, err := artifact.Message().Marshal()

		if err != nil {
			return FeatureFrameSize + 1
		}

		return len(wire)
	}

	low := headerFloats
	high := FeatureFrameSize / float64WireBytes

	for low < high {
		mid := (low + high + 1) / 2

		if probe(mid) <= FeatureFrameSize {
			low = mid
			continue
		}

		high = mid - 1
	}

	return low
}

/*
TrimOldestFloats keeps the newest maxCount samples.
*/
func TrimOldestFloats(values []float64, maxCount int) []float64 {
	if maxCount <= 0 || len(values) <= maxCount {
		return values
	}

	return values[len(values)-maxCount:]
}

/*
TrimLargestFloats drops the largest values until the slice fits maxCount.
*/
func TrimLargestFloats(values []float64, maxCount int) []float64 {
	if maxCount <= 0 {
		return nil
	}

	if len(values) <= maxCount {
		return values
	}

	indices := make([]int, len(values))

	for index := range indices {
		indices[index] = index
	}

	sort.Slice(indices, func(left, right int) bool {
		return math.Abs(values[indices[left]]) > math.Abs(values[indices[right]])
	})

	keep := make(map[int]struct{}, maxCount)

	for _, index := range indices[len(indices)-maxCount:] {
		keep[index] = struct{}{}
	}

	trimmed := make([]float64, 0, maxCount)

	for index, value := range values {
		if _, ok := keep[index]; ok {
			trimmed = append(trimmed, value)
		}
	}

	return trimmed
}

/*
TrimHistoryTails trims parallel history slices until their total length fits maxFloats.
*/
func TrimHistoryTails(histories [][]float64, maxFloats int) [][]float64 {
	trimmed := make([][]float64, len(histories))

	for index, history := range histories {
		trimmed[index] = append([]float64(nil), history...)
	}

	total := 0

	for _, history := range trimmed {
		total += len(history)
	}

	for total > maxFloats {
		longestIndex := -1
		longestLength := 0

		for index, history := range trimmed {
			if len(history) > longestLength {
				longestLength = len(history)
				longestIndex = index
			}
		}

		if longestIndex < 0 || longestLength == 0 {
			break
		}

		trimmed[longestIndex] = trimmed[longestIndex][1:]
		total--
	}

	return trimmed
}

/*
TouchSpread estimates spread in basis points from a trade price window.
*/
func TouchSpread(prices []float64) (float64, bool) {
	if len(prices) < 2 {
		return 0, false
	}

	minPrice := prices[0]
	maxPrice := prices[0]

	for _, price := range prices[1:] {
		if price < minPrice {
			minPrice = price
		}

		if price > maxPrice {
			maxPrice = price
		}
	}

	mid := (minPrice + maxPrice) / 2

	if mid <= 0 || maxPrice <= minPrice {
		return 0, false
	}

	return (maxPrice - minPrice) / mid * 10000, true
}

/*
AnchorChange returns signed move and precursor from anchor to latest price.
*/
func AnchorChange(anchor, latest float64) (move, precursor float64) {
	if anchor <= 0 || latest <= 0 {
		return 0, 0
	}

	move = (latest - anchor) / anchor
	precursor = move

	return move, precursor
}
