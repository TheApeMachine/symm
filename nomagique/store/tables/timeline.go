package tables

import (
	"context"
	"iter"
	"slices"

	"github.com/apache/iceberg-go"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
Timeline reconstructs the sequential market tape from the ticker's perspective,
attaching all concurrent trades, level3 events, and signal measurements into ticker.Peers.
It streams using pull iterators to maintain constant O(1) batch memory.
*/
func (catalog *Catalog) Timeline(
	ctx context.Context,
	epoch int64,
	symbol string,
	fromTick int64,
	toTick int64,
) iter.Seq[*data.Measurement[float64]] {
	var filter iceberg.BooleanExpression

	if symbol != "" {
		filter = iceberg.EqualTo(iceberg.Reference("symbol"), symbol)
	}

	if fromTick > 0 {
		expression := iceberg.BooleanExpression(iceberg.GreaterThanEqual(iceberg.Reference("tick"), fromTick))

		if filter != nil {
			expression = iceberg.NewAnd(filter, expression)
		}

		filter = expression
	}

	if toTick > 0 && toTick >= fromTick {
		expression := iceberg.BooleanExpression(iceberg.LessThanEqual(iceberg.Reference("tick"), toTick))

		if filter != nil {
			expression = iceberg.NewAnd(filter, expression)
		}

		filter = expression
	}

	return func(yield func(*data.Measurement[float64]) bool) {
		tickerSeq := catalog.Scan(ctx, SpotTicker, epoch, filter, 0)
		tradeSeq := catalog.Scan(ctx, SpotTrade, epoch, filter, 0)
		level3Seq := catalog.Scan(ctx, SpotLevel3, epoch, filter, 0)
		measurementSeq := catalog.Scan(ctx, Measurements, epoch, filter, 0)

		nextTrade, stopTrade := iter.Pull(tradeSeq)
		defer stopTrade()

		nextLevel3, stopLevel3 := iter.Pull(level3Seq)
		defer stopLevel3()

		nextMeasurement, stopMeasurement := iter.Pull(measurementSeq)
		defer stopMeasurement()

		currentTrade, hasTrade := nextTrade()
		currentLevel3, hasLevel3 := nextLevel3()
		currentMeasurement, hasMeasurement := nextMeasurement()

		for ticker := range tickerSeq {
			for hasTrade && currentTrade.SeqIdx <= ticker.SeqIdx {
				ticker.Peers = append(ticker.Peers, currentTrade)
				currentTrade, hasTrade = nextTrade()
			}

			for hasLevel3 && currentLevel3.SeqIdx <= ticker.SeqIdx {
				ticker.Peers = append(ticker.Peers, currentLevel3)
				currentLevel3, hasLevel3 = nextLevel3()
			}

			for hasMeasurement && currentMeasurement.SeqIdx <= ticker.SeqIdx {
				ticker.Peers = append(ticker.Peers, currentMeasurement)
				currentMeasurement, hasMeasurement = nextMeasurement()
			}

			if !yield(ticker) {
				return
			}
		}
	}
}

/*
Symbols discovers distinct symbols present in an epoch by reading only the symbol column.
*/
func (catalog *Catalog) Symbols(ctx context.Context, epoch int64) ([]string, error) {
	symbolSet := make(map[string]struct{})

	for measurement := range catalog.Scan(ctx, SpotTicker, epoch, nil, 0, "symbol") {
		if measurement.Label != "" {
			symbolSet[measurement.Label] = struct{}{}
		}
	}

	symbols := make([]string, 0, len(symbolSet))

	for symbol := range symbolSet {
		symbols = append(symbols, symbol)
	}

	slices.Sort(symbols)

	return symbols, nil
}
