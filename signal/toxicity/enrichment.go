package toxicity

import (
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/statutil"
)

/*
orderFlow carries the L3-derived cancel-vs-fill evidence for one symbol over the
cadence window. cancelBid/cancelAsk are the order quantities deleted WITHOUT a
matching trade at that price (genuine pulls); fillBid/fillAsk are deletes that
DID coincide with a trade at the level (liquidity hit, not pulled). l3 reports
whether any level3 event was seen at all — when false the signal must degrade
honestly to L2 and must NOT label cancel-vs-fill it cannot derive.
*/
type orderFlow struct {
	cancelBid, cancelAsk float64
	fillBid, fillAsk     float64
	l3                   bool
}

func (flow orderFlow) cancelTotal() float64 {
	return flow.cancelBid + flow.cancelAsk
}

func (flow orderFlow) fillTotal() float64 {
	return flow.fillBid + flow.fillAsk
}

/*
asymmetry reports how one-sided the cancels are (one side retreating). 0 means
both sides pulled equally; 1 means one side pulled entirely.
*/
func (flow orderFlow) asymmetry() float64 {
	total := flow.cancelTotal()

	if total <= 0 {
		return 0
	}

	return math.Abs(flow.cancelBid-flow.cancelAsk) / total
}

/*
level3Flow replays per-order add/delete events in the cadence window and labels
each delete as cancel (no trade at that price) or fill (trade present at that
price). The window start derives from observed cadence on the supplied stamps
(statutil.MedianCadence × WindowDepth), never a fixed horizon. There is no
historical L3 backfill, so an empty seek leaves flow.l3 false and the caller
falls back to L2-only degradation.
*/
func (signal *Signal) level3Flow(
	symbol string,
	windowStamps []float64,
	currentStamp float64,
) orderFlow {
	flow := orderFlow{}

	if signal.tree == nil {
		return flow
	}

	windowStart := windowStartFromCadence(windowStamps, currentStamp)
	trades := signal.tradePrices(symbol, windowStart, currentStamp)

	for artifact := range signal.tree.Seek([]byte("level3/")) {
		stamp := float64(artifact.Timestamp())

		if currentStamp > 0 && stamp > currentStamp {
			break
		}

		if windowStart > 0 && stamp < windowStart {
			continue
		}

		accumulateOrderEvents(artifact, symbol, trades, &flow)
	}

	return flow
}

/*
accumulateOrderEvents folds one L3 frame's bid/ask delete events into the flow,
correlating each delete price against the trade tape to split cancel from fill.
*/
func accumulateOrderEvents(
	artifact *datura.Artifact,
	symbol string,
	trades []float64,
	flow *orderFlow,
) {
	for rowIndex := 0; ; rowIndex++ {
		rowSymbol := datura.Peek[string](artifact, "data", rowIndex, "symbol")

		if rowSymbol == "" {
			return
		}

		if rowSymbol != symbol {
			continue
		}

		flow.l3 = true
		foldSide(artifact, rowIndex, "bids", trades, &flow.cancelBid, &flow.fillBid)
		foldSide(artifact, rowIndex, "asks", trades, &flow.cancelAsk, &flow.fillAsk)
	}
}

func foldSide(
	artifact *datura.Artifact,
	rowIndex int,
	side string,
	trades []float64,
	cancel, fill *float64,
) {
	for orderIndex := 0; ; orderIndex++ {
		event := datura.Peek[string](artifact, "data", rowIndex, side, orderIndex, "event")

		if event == "" {
			return
		}

		if event != "delete" {
			continue
		}

		price := datura.Peek[float64](artifact, "data", rowIndex, side, orderIndex, "limit_price")
		quantity := datura.Peek[float64](artifact, "data", rowIndex, side, orderIndex, "order_qty")

		if quantity <= 0 {
			continue
		}

		if tradedAt(price, trades) {
			*fill += quantity

			continue
		}

		*cancel += quantity
	}
}

/*
tradePrices collects executed trade prices for one symbol in the window so a
delete can be tested for a coincident fill. A delete at a price that the tape
also printed is liquidity hit (fill); a delete with no matching print is a pull
(cancel) — toxicity's core metric, which L2 qty deltas cannot distinguish.
*/
func (signal *Signal) tradePrices(symbol string, windowStart, currentStamp float64) []float64 {
	prices := []float64{}

	for artifact := range signal.tree.Seek([]byte("trade/")) {
		stamp := float64(artifact.Timestamp())

		if currentStamp > 0 && stamp > currentStamp {
			break
		}

		if windowStart > 0 && stamp < windowStart {
			continue
		}

		for rowIndex := 0; ; rowIndex++ {
			rowSymbol := datura.Peek[string](artifact, "data", rowIndex, "symbol")

			if rowSymbol == "" {
				break
			}

			if rowSymbol != symbol {
				continue
			}

			price := datura.Peek[float64](artifact, "data", rowIndex, "price")

			if price > 0 {
				prices = append(prices, price)
			}
		}
	}

	return prices
}

/*
tradedAt reports whether the tape printed at a delete's price level, with the
match tolerance derived from the dispersion of observed trade prices (median
absolute deviation) rather than a hardcoded epsilon.
*/
func tradedAt(price float64, trades []float64) bool {
	if price <= 0 || len(trades) == 0 {
		return false
	}

	tolerance := statutil.MedianAbsoluteDeviation(trades, statutil.Median(trades))

	for _, traded := range trades {
		if math.Abs(traded-price) <= tolerance {
			return true
		}
	}

	return false
}

func windowStartFromCadence(windowStamps []float64, currentStamp float64) float64 {
	if len(windowStamps) == 0 || currentStamp <= 0 {
		return 0
	}

	cadence := statutil.MedianCadence(windowStamps)
	depth := statutil.WindowDepth(windowStamps)

	if cadence <= 0 || depth <= 0 {
		return windowStamps[0]
	}

	return currentStamp - cadence*float64(depth)
}
