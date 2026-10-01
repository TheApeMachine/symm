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
		futuresTickerSeq := catalog.Scan(ctx, FuturesTicker, epoch, filter, 0)
		futuresTradeSeq := catalog.Scan(ctx, FuturesTrade, epoch, filter, 0)
		measurementSeq := catalog.Scan(ctx, Measurements, epoch, filter, 0)

		nextTicker, stopTicker := iter.Pull(tickerSeq)
		defer stopTicker()

		firstTicker, hasTicker := nextTicker()

		nextTrade, stopTrade := iter.Pull(tradeSeq)
		defer stopTrade()

		nextLevel3, stopLevel3 := iter.Pull(level3Seq)
		defer stopLevel3()

		nextFuturesTicker, stopFuturesTicker := iter.Pull(futuresTickerSeq)
		defer stopFuturesTicker()

		nextFuturesTrade, stopFuturesTrade := iter.Pull(futuresTradeSeq)
		defer stopFuturesTrade()

		nextMeasurement, stopMeasurement := iter.Pull(measurementSeq)
		defer stopMeasurement()

		currentTrade, hasTrade := nextTrade()
		currentLevel3, hasLevel3 := nextLevel3()
		currentFuturesTicker, hasFuturesTicker := nextFuturesTicker()
		currentFuturesTrade, hasFuturesTrade := nextFuturesTrade()
		currentMeasurement, hasMeasurement := nextMeasurement()

		attachPeers := func(frame *data.Measurement[float64]) {
			for hasTrade && currentTrade.SeqIdx <= frame.SeqIdx {
				frame.Peers = append(frame.Peers, currentTrade)
				currentTrade, hasTrade = nextTrade()
			}

			for hasLevel3 && currentLevel3.SeqIdx <= frame.SeqIdx {
				frame.Peers = append(frame.Peers, currentLevel3)
				currentLevel3, hasLevel3 = nextLevel3()
			}

			for hasFuturesTicker && currentFuturesTicker.SeqIdx <= frame.SeqIdx {
				frame.Peers = append(frame.Peers, currentFuturesTicker)
				currentFuturesTicker, hasFuturesTicker = nextFuturesTicker()
			}

			for hasFuturesTrade && currentFuturesTrade.SeqIdx <= frame.SeqIdx {
				frame.Peers = append(frame.Peers, currentFuturesTrade)
				currentFuturesTrade, hasFuturesTrade = nextFuturesTrade()
			}

			for hasMeasurement && currentMeasurement.SeqIdx <= frame.SeqIdx {
				// When the primary tape is itself a Measurements row (legacy
				// mis-routed venue quotes), do not attach the frame to itself.
				if currentMeasurement.SeqIdx == frame.SeqIdx &&
					currentMeasurement.Source == frame.Source &&
					currentMeasurement.Label == frame.Label {
					currentMeasurement, hasMeasurement = nextMeasurement()
					continue
				}

				frame.Peers = append(frame.Peers, currentMeasurement)
				currentMeasurement, hasMeasurement = nextMeasurement()
			}
		}

		if hasTicker {
			frame := firstTicker
			for {
				attachPeers(frame)

				if !yield(frame) {
					return
				}

				next, ok := nextTicker()
				if !ok {
					return
				}
				frame = next
			}
		}

		// Legacy epochs: venue quotes were drained into Measurements because
		// deriveChannel required venue=true. Materialize the measurement
		// stream once so quote primaries still receive concurrent signal
		// peers that share earlier ticks — never invent levels.
		pending := make([]*data.Measurement[float64], 0)

		if hasMeasurement {
			pending = append(pending, currentMeasurement)

			for {
				row, ok := nextMeasurement()
				if !ok {
					break
				}
				pending = append(pending, row)
			}
		}

		peerIdx := 0

		for _, frame := range pending {
			if !isQuoteTapeFrame(frame) {
				continue
			}

			attachPeers(frame)

			for peerIdx < len(pending) && pending[peerIdx].SeqIdx <= frame.SeqIdx {
				peer := pending[peerIdx]
				peerIdx++

				if peer == frame {
					continue
				}

				frame.Peers = append(frame.Peers, peer)
			}

			if !yield(frame) {
				return
			}
		}
	}
}

/*
isQuoteTapeFrame reports whether a measurement carries an honest market quote
usable as a Timeline primary frame (bid+ask mid or last).
*/
func isQuoteTapeFrame(measurement *data.Measurement[float64]) bool {
	if measurement == nil {
		return false
	}

	_, hasBid := measurement.LookupMetric("bid")
	_, hasAsk := measurement.LookupMetric("ask")

	if hasBid && hasAsk {
		return true
	}

	_, hasLast := measurement.LookupMetric("last")
	_, hasPrice := measurement.LookupMetric("price")
	_, hasMid := measurement.LookupMetric("mid")

	return hasLast || hasPrice || hasMid
}

/*
Symbols discovers distinct symbols present in an epoch by reading only the symbol column.
*/
func (catalog *Catalog) Symbols(ctx context.Context, epoch int64) ([]string, error) {
	symbolSet := make(map[string]struct{})

	collect := func(tableName string) {
		for measurement := range catalog.Scan(ctx, tableName, epoch, nil, 0, "symbol") {
			if measurement.Label != "" {
				symbolSet[measurement.Label] = struct{}{}
			}
		}
	}

	collect(SpotTicker)

	// Legacy mis-routed venue tape lives under Measurements.
	if len(symbolSet) == 0 {
		collect(Measurements)
		collect(SpotTrade)
	}

	symbols := make([]string, 0, len(symbolSet))

	for symbol := range symbolSet {
		symbols = append(symbols, symbol)
	}

	slices.Sort(symbols)

	return symbols, nil
}
