package probe_test

import (
	"context"
	"encoding/binary"
	"math"
	"testing"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
	. "github.com/theapemachine/symm/signal"
	"github.com/theapemachine/symm/signal/liquidity"
)

func encode(samples ...float64) []byte {
	payload := make([]byte, 8*len(samples))
	for i, s := range samples {
		binary.BigEndian.PutUint64(payload[i*8:(i+1)*8], math.Float64bits(s))
	}
	return payload
}

func insertFeatures(signal *liquidity.Signal, scope string, samples ...float64) {
	payload := encode(samples...)
	artifact := datura.Acquire("depth-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(payload)
	InsertTreeArtifact(signal.tree, artifact)
	artifact.Release()
}

func depthPayload(scaledQuoteVol float64, peers []float64, relativeVolume float64, baselineReady bool) []float64 {
	samples := []float64{scaledQuoteVol, float64(len(peers))}
	samples = append(samples, peers...)
	samples = append(samples, relativeVolume)
	baselineFlag := 0.0
	if baselineReady {
		baselineFlag = 1
	}
	samples = append(samples, baselineFlag)
	return samples
}

func query(scope string) datura.Artifact {
	q := datura.Acquire("trader", datura.Artifact_Type_json)
	q.WithRole("measurement")
	q.WithScope(scope)
	return *q
}

func TestOnePeer(t *testing.T) {
	signal := liquidity.NewSignal(context.Background(), qpool.NewQ[any](context.Background(), 2, 4, nil), NewTestTree())
	insertFeatures(signal, "SOLO/EUR", depthPayload(100, []float64{200}, 1, false)...)
	result := signal.Measure(query("SOLO/EUR"))
	t.Logf("1 peer: nil=%v cat=%d conf=%v surprise=%v", result == nil, datura.Peek[int](result, "classifier.category"), datura.Peek[float64](result, "classifier.confidence"), datura.Peek[float64](result, "surprise"))
}

func TestZeroPeersInPayload(t *testing.T) {
	signal := liquidity.NewSignal(context.Background(), qpool.NewQ[any](context.Background(), 2, 4, nil), NewTestTree())
	insertFeatures(signal, "SOLO/EUR", depthPayload(100, []float64{}, 1, false)...)
	result := signal.Measure(query("SOLO/EUR"))
	t.Logf("0 peers: nil=%v cat=%d conf=%v", result == nil, datura.Peek[int](result, "classifier.category"), datura.Peek[float64](result, "classifier.confidence"))
}

func TestTwoPeersSurprise(t *testing.T) {
	signal := liquidity.NewSignal(context.Background(), qpool.NewQ[any](context.Background(), 2, 4, nil), NewTestTree())
	insertFeatures(signal, "ALT/EUR", depthPayload(1200, []float64{800, 900}, 1, false)...)
	result := signal.Measure(query("ALT/EUR"))
	t.Logf("2 peers: nil=%v cat=%d conf=%v surprise=%v", result == nil, datura.Peek[int](result, "classifier.category"), datura.Peek[float64](result, "classifier.confidence"), datura.Peek[float64](result, "surprise"))
}
