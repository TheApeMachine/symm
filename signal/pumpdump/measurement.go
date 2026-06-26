package pumpdump

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
)

func writeMeasurement(
	measurement *datura.Artifact,
	sample tickerSample,
	book bookSnapshot,
	trades tradeSnapshot,
	metrics ignitionMetrics,
) {
	measurement.MergeOutput("rvol", metrics.rvol)
	measurement.MergeOutput("precursor", metrics.precursor)
	measurement.MergeOutput("compression", metrics.compression)
	measurement.MergeOutput("rvolDecline", metrics.rvolDecline)
	measurement.MergeOutput("spread", sample.spread)
	measurement.MergeOutput("bookCompression", metrics.bookCompression)
	measurement.MergeOutput("peerRvol", metrics.peerRvol)
	measurement.MergeOutput("peerPrecursor", metrics.peerPrecursor)

	measurement.Merge("volume", sample.volume)
	measurement.Merge("last", sample.last)
	measurement.Merge("spread", sample.spread)
	measurement.Merge("bookSpread", book.spread)
	measurement.Merge("touchDepth", book.touchDepth)
	measurement.Merge("tradeVolume", trades.volume)
	measurement.Merge("volumeDelta", metrics.volumeDelta)
	measurement.Merge("logReturn", metrics.logReturn)
	measurement.Merge("legAnchorLow", metrics.legAnchorLow)
	measurement.Merge("legAnchorHigh", metrics.legAnchorHigh)
	measurement.Merge("thinBook", metrics.thinBook)
	measurement.Merge("timestamp", sample.stamp)
}

/*
recordExhaustion stamps lastExhaustionStamp when Faded Exhaustion dominates this
frame, so the next measurement resets local leg anchors and a new leg starts
fresh. Otherwise the prior leg's stamp is carried forward unchanged.
*/
func recordExhaustion(
	measurement *datura.Artifact,
	metrics ignitionMetrics,
	sample tickerSample,
) {
	exhaustionIndex := float64(logic.CategoryIndex(logic.CategoryFadedExhaustion))

	if datura.Peek[float64](measurement, "output", "category") == exhaustionIndex {
		measurement.Merge("lastExhaustionStamp", sample.stamp)

		return
	}

	measurement.Merge("lastExhaustionStamp", metrics.exhaustionStamp)
}

func readTickerSample(datapoint *datura.Artifact, rowIndex int) (tickerSample, bool) {
	symbol := datura.Peek[string](datapoint, "data", rowIndex, "symbol")

	if symbol == "" {
		return tickerSample{}, false
	}

	bid := datura.Peek[float64](datapoint, "data", rowIndex, "bid")
	ask := datura.Peek[float64](datapoint, "data", rowIndex, "ask")

	return tickerSample{
		symbol:   symbol,
		bid:      bid,
		ask:      ask,
		last:     datura.Peek[float64](datapoint, "data", rowIndex, "last"),
		volume:   datura.Peek[float64](datapoint, "data", rowIndex, "volume"),
		change:   datura.Peek[float64](datapoint, "data", rowIndex, "change"),
		changePC: datura.Peek[float64](datapoint, "data", rowIndex, "change_pct"),
		spread:   ask - bid,
		stamp:    float64(datapoint.Timestamp()),
	}, true
}

/*
invalidInvariant reports why a ticker row cannot be scored, and whether that
reason is anomalous. A missing last/volume is normal for an illiquid pair that
has not traded — skip it quietly. A crossed book (ask < bid) is a genuine data
anomaly worth logging.
*/
func (sample tickerSample) invalidInvariant() (reason string, anomalous bool) {
	if sample.last <= 0 {
		return "last", false
	}

	if sample.volume <= 0 {
		return "volume", false
	}

	if sample.ask < sample.bid {
		return "ask < bid", true
	}

	return "", false
}

func logInvalidRow(symbol, invariant string) {
	errnie.Error(errnie.Err(errnie.Validation, "pumpdump: invalid ticker row", nil).With(
		"symbol", symbol,
		"invariant", invariant,
	))
}
