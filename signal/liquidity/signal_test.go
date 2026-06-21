package liquidity

import (
	"context"
	"testing"

	"github.com/theapemachine/qpool"
)

func newTestPool(testingTB testing.TB) *qpool.Q[any] {
	if testingTB != nil {
		testingTB.Helper()
	}

	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)

	if pool == nil && testingTB != nil {
		testingTB.Fatal("qpool.NewQ returned nil")
	}

	return pool
}

func depthFeaturesPayload(
	scaledQuoteVol float64,
	peers []float64,
	relativeVolume float64,
	baselineReady bool,
) []float64 {
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
