package conditions

import (
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/tests"
	instrumentfixture "github.com/theapemachine/symm/tests/fixtures/instrument"
)

/*
BookPath emits complete synthetic Kraken book snapshots from explicit depth
profiles. It is a producer only: package-local tests decide what each path means.
*/
func BookPath(
	bidQuantities [][]float64,
	askQuantities [][]float64,
	spreadTicks []int,
	stamps []time.Time,
) *tests.Market {
	frameCount := len(bidQuantities)

	if frameCount == 0 || len(askQuantities) != frameCount ||
		len(spreadTicks) != frameCount || len(stamps) != frameCount {
		panic(errnie.Err(errnie.Validation, "conditions: aligned book path required", nil))
	}

	payloads := make([][]byte, frameCount)

	for index := range frameCount {
		payloads[index] = depthBookPayload(
			bidQuantities[index], askQuantities[index], spreadTicks[index], stamps[index],
		)
	}

	return tests.NewMarket().
		Prefix(instrumentfixture.NewFixture(instrumentfixture.SNAPSHOT, 1)).
		Feed(tests.NewStaticSequence(payloads...))
}

/*
depthBookPayload encodes one full two-sided depth profile around a stable midpoint.
*/
func depthBookPayload(
	bidQuantities []float64,
	askQuantities []float64,
	spreadTicks int,
	at time.Time,
) []byte {
	if len(bidQuantities) == 0 || len(askQuantities) == 0 ||
		spreadTicks < 1 || at.IsZero() {
		panic(errnie.Err(errnie.Validation, "conditions: valid book snapshot required", nil))
	}

	const tickSize = 0.0001
	const midpointTicks = 5667
	bestBidTick := midpointTicks - spreadTicks/2
	bestAskTick := bestBidTick + spreadTicks
	bids := make([]map[string]any, len(bidQuantities))
	asks := make([]map[string]any, len(askQuantities))

	for index, quantity := range bidQuantities {
		if quantity < 0 {
			panic(errnie.Err(errnie.Validation, "conditions: non-negative bid quantity required", nil))
		}

		bids[index] = map[string]any{
			"price": float64(bestBidTick-index) * tickSize,
			"qty":   quantity,
		}
	}

	for index, quantity := range askQuantities {
		if quantity < 0 {
			panic(errnie.Err(errnie.Validation, "conditions: non-negative ask quantity required", nil))
		}

		asks[index] = map[string]any{
			"price": float64(bestAskTick+index) * tickSize,
			"qty":   quantity,
		}
	}

	return tests.MarshalFrame(map[string]any{
		"channel": "book",
		"type":    "snapshot",
		"data": []any{map[string]any{
			"symbol":    subjectSymbol,
			"bids":      bids,
			"asks":      asks,
			"timestamp": at.UTC().Format(time.RFC3339Nano),
		}},
	})
}
