package conditions

import (
	"time"

	"github.com/theapemachine/symm/tests"
	instrumentfixture "github.com/theapemachine/symm/tests/fixtures/instrument"
)

/*
TradePath produces an ordered Kraken trade stream from explicit executed
prices, quantities, aggressor sides, and timestamps. It keeps trade-flow tests
independent from ticker summaries and from the signal equations under test.
*/
func TradePath(
	prices []float64,
	quantities []float64,
	sides []string,
	stamps []time.Time,
) *tests.Market {
	pathLength := len(prices)

	if pathLength == 0 || pathLength != len(quantities) ||
		pathLength != len(sides) || pathLength != len(stamps) {
		panic("conditions: trade paths must be non-empty and equally sized")
	}

	payloads := make([][]byte, 0, pathLength)

	for index, price := range prices {
		if price <= 0 || quantities[index] <= 0 ||
			(sides[index] != "buy" && sides[index] != "sell") ||
			stamps[index].IsZero() {
			panic("conditions: trade price, quantity, side, and timestamp are invalid")
		}

		if index > 0 && stamps[index].Before(stamps[index-1]) {
			panic("conditions: trade timestamps must not move backward")
		}

		payloads = append(payloads, tradePayload(
			subjectSymbol, index, stamps[index], price, quantities[index], sides[index],
		))
	}

	return tests.NewMarket().
		Prefix(instrumentfixture.NewFixture(instrumentfixture.SNAPSHOT, 1)).
		Feed(tests.NewStaticSequence(payloads...))
}

/*
tradePayload emits one executed trade payload shared by multi-stream and
trade-only synthetic market paths.
*/
func tradePayload(
	symbol string,
	index int,
	at time.Time,
	price float64,
	quantity float64,
	side string,
) []byte {
	return tests.MarshalFrame(map[string]any{
		"channel": "trade",
		"type":    "update",
		"data": []map[string]any{{
			"symbol":    symbol,
			"side":      side,
			"price":     price,
			"qty":       quantity,
			"ord_type":  "market",
			"trade_id":  index + 1,
			"timestamp": at,
		}},
	})
}
