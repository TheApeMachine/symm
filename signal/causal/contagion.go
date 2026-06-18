package causal

import (
	"encoding/binary"
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/algorithm"
)

type contagionStage struct {
	artifact *datura.Artifact
	value    float64
}

func newContagionStage() *contagionStage {
	return &contagionStage{
		artifact: datura.Acquire("causal-contagion", datura.Artifact_Type_json),
	}
}

func (stage *contagionStage) Write(payload []byte) (int, error) {
	return len(payload), nil
}

func (stage *contagionStage) Read(payload []byte) (int, error) {
	putFloat64Payload(&stage.artifact, "causal-contagion", stage.value)

	return stage.artifact.Read(payload)
}

func putFloat64Payload(artifact **datura.Artifact, name string, value float64) {
	*artifact = datura.Acquire(name, datura.Artifact_Type_json)
	encoded := make([]byte, 8)
	binary.BigEndian.PutUint64(encoded, math.Float64bits(value))
	_ = (*artifact).SetPayload(encoded)
}

func (stage *contagionStage) Close() error {
	return nil
}

func peakAbsPayload(raw []byte) float64 {
	if len(raw) < 8 || len(raw)%8 != 0 {
		return 0
	}

	peak := 0.0

	for index := 0; index < len(raw); index += 8 {
		sample := math.Abs(math.Float64frombits(binary.BigEndian.Uint64(raw[index : index+8])))

		if sample > peak {
			peak = sample
		}
	}

	return peak
}

func peakNodeMagnitude(nodes *algorithm.NodeRing) float64 {
	streams := nodes.Streams()

	if len(streams) < 3 || len(streams[0]) == 0 {
		return 0
	}

	peak := 0.0
	rowIndex := len(streams[0]) - 1

	for nodeIndex, stream := range streams {
		if nodeIndex == nodeMacro || nodeIndex == nodeTarget {
			continue
		}

		if rowIndex >= len(stream) {
			continue
		}

		sample := math.Abs(stream[rowIndex])

		if sample > peak {
			peak = sample
		}
	}

	return peak
}
