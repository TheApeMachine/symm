package codec

import (
	"encoding/binary"
	"io"
	"math"
	"sort"

	"github.com/theapemachine/datura"
)

const (
	maxFeatureFloatCount = 4096
	FeatureFrameSize     = maxFeatureFloatCount * 8
)

/*
EncodePayload serializes float64 samples as big-endian IEEE754 bytes.
*/
func EncodePayload(samples ...float64) []byte {
	payload := make([]byte, 8*len(samples))

	for index, sample := range samples {
		binary.BigEndian.PutUint64(payload[index*8:], math.Float64bits(sample))
	}

	return payload
}

/*
ReadFeatureArtifact copies the artifact payload into buffer.
*/
func ReadFeatureArtifact(buffer []byte, artifact *datura.Artifact) (int, error) {
	if artifact == nil {
		return 0, io.EOF
	}

	payload, err := artifact.DecryptPayload()

	if err != nil {
		return 0, err
	}

	if len(buffer) < len(payload) {
		return 0, io.ErrShortBuffer
	}

	copy(buffer, payload)

	return len(payload), io.EOF
}

/*
MaxFeatureFloats returns the maximum float count for a feature payload.
*/
func MaxFeatureFloats(name, role, scope string, headerFloats int) int {
	if headerFloats >= maxFeatureFloatCount {
		return maxFeatureFloatCount
	}

	_ = name
	_ = role
	_ = scope

	return maxFeatureFloatCount
}

/*
TrimLargestFloats keeps the smallest maxCount values.
*/
func TrimLargestFloats(values []float64, maxCount int) []float64 {
	if maxCount <= 0 {
		return nil
	}

	if len(values) <= maxCount {
		return values
	}

	indexed := make([]int, len(values))

	for index := range values {
		indexed[index] = index
	}

	sort.Slice(indexed, func(left, right int) bool {
		if values[indexed[left]] == values[indexed[right]] {
			return indexed[left] < indexed[right]
		}

		return values[indexed[left]] < values[indexed[right]]
	})

	trimmed := make([]float64, maxCount)

	for index := range maxCount {
		trimmed[index] = values[indexed[index]]
	}

	return trimmed
}

/*
TrimHistoryTails shortens variable-length histories to fit maxTotal floats.
*/
func TrimHistoryTails(histories [][]float64, maxTotal int) [][]float64 {
	if maxTotal <= 0 {
		return histories
	}

	totalLength := 0

	for _, history := range histories {
		totalLength += len(history)
	}

	if totalLength <= maxTotal {
		return histories
	}

	trimmed := make([][]float64, len(histories))
	remaining := maxTotal

	for index, history := range histories {
		if remaining <= 0 {
			trimmed[index] = nil

			continue
		}

		if len(history) <= remaining {
			trimmed[index] = history
			remaining -= len(history)

			continue
		}

		trimmed[index] = append([]float64(nil), history[len(history)-remaining:]...)
		remaining = 0
	}

	return trimmed
}
