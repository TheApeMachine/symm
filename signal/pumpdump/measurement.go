package pumpdump

import (
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/algo"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

var (
	dimensionless = nmtypes.Descriptor{
		Unit:      nmtypes.UnitDimensionless,
		Timescale: nmtypes.TimescaleInstantaneous,
	}
	price = nmtypes.Descriptor{
		Unit:      nmtypes.UnitPrice,
		Timescale: nmtypes.TimescaleInstantaneous,
	}
	baseQuantity = nmtypes.Descriptor{
		Unit:      nmtypes.UnitBaseCurrency,
		Timescale: nmtypes.TimescaleInstantaneous,
	}
	baseRate = nmtypes.Descriptor{
		Unit:      nmtypes.UnitBaseCurrencyPerSecond,
		Timescale: nmtypes.TimescalePerSecond,
	}
)

/*
measurement projects the current three-branch PumpDump state without
recomputing signal math. A metric is present only after its authoritative
stream has produced it; provisional measurements therefore remain truthful.
*/
func (signal *Signal) measurement(
	at time.Time,
	output nomagique.Frame,
) *nmtypes.Measurement {
	observedFrom := pumpDumpObservedFrom(output, at)
	measurement := nmtypes.NewMeasurement(
		uuid.NewString(),
		signal.Name(),
		at.UnixNano(),
		observedFrom.UnixNano(),
	)
	measurement.At = at
	measurement.ObservedFrom = observedFrom
	measurement.Horizon = at.Sub(observedFrom)
	measurement.Maturity = frameNumber(output, algo.SymbolMaturity)
	measurement.Metadata = make(map[string]float64)

	putMetadata(measurement, output, "event", algo.SymbolPumpDumpEvent)
	putMetadata(measurement, output, "capacity", algo.SymbolCapacity)
	putMetadata(measurement, output, "trade_quantity", algo.SymbolTradeQuantity)
	putMetadata(measurement, output, "trade_price", algo.SymbolTradePrice)
	putMetadata(measurement, output, "ticker_last", algo.SymbolAnchorLast)
	putMetadata(measurement, output, "vwap", algo.SymbolVWAP)
	putMetadata(measurement, output, "reported_volume", algo.SymbolReportedVolume)
	putMetadata(measurement, output, "best_bid", algo.SymbolBid)
	putMetadata(measurement, output, "best_ask", algo.SymbolAsk)
	putMetadata(measurement, output, "bid_depth", algo.SymbolLadderBidDepth)
	putMetadata(measurement, output, "ask_depth", algo.SymbolLadderAskDepth)
	putMetadata(measurement, output, "unix_sec", algo.SymbolUnixSec)
	putMetadata(measurement, output, "unix_nsec", algo.SymbolUnixNsec)
	putMetadata(measurement, output, "bar_rate", algo.SymbolIgnitionBarRate)
	putMetadata(measurement, output, "rate_baseline", algo.SymbolIgnitionRateBaseline)
	putMetadata(measurement, output, "anchor_fast_baseline", algo.SymbolAnchorFastBaseline)
	putMetadata(measurement, output, "anchor_slow_baseline", algo.SymbolAnchorSlowBaseline)
	putMetadata(measurement, output, "anchor_dispersion", algo.SymbolAnchorDispersion)
	putMetadata(measurement, output, "spread_baseline", algo.SymbolLadderSpreadBaseline)
	putMetadata(measurement, output, "bid_depth_delta", algo.SymbolLadderBidDelta)
	putMetadata(measurement, output, "ask_depth_delta", algo.SymbolLadderAskDelta)

	ignitionReady := frameNumber(output, algo.SymbolIgnitionClassified) != 0
	anchorReady := frameNumber(output, algo.SymbolAnchorReady) != 0
	ladderReady := frameNumber(output, algo.SymbolLadderReady) != 0
	putFrameMetric(measurement, output, algo.SymbolBid,
		types.MetricKey(types.MetricBestPrice, types.SideBuy), price)
	putFrameMetric(measurement, output, algo.SymbolAsk,
		types.MetricKey(types.MetricBestPrice, types.SideSell), price)
	putFrameMetric(measurement, output, algo.SymbolMidpoint,
		string(types.MetricMidpoint), price)
	putFrameMetric(measurement, output, algo.SymbolTradePrice,
		string(types.MetricTradePrice), price)
	putFrameMetric(measurement, output, algo.SymbolTradeQuantity,
		string(types.MetricTradeQuantity), baseQuantity)
	putNormalizedFrameMetric(measurement, output, algo.SymbolRVOL,
		algo.SymbolRVOLNormalized, string(types.MetricRVOL), baseRate, ignitionReady)
	putFrameMetric(measurement, output, algo.SymbolRVOLLift,
		string(types.MetricRVOLLift), dimensionless)
	putNormalizedFrameMetric(measurement, output, algo.SymbolSpread,
		algo.SymbolSpreadNormalized, string(types.MetricSpread), price, true)
	putFrameMetric(measurement, output, algo.SymbolAnchorDetach,
		string(types.MetricAnchorDetach), dimensionless)
	putFrameMetric(measurement, output, algo.SymbolAnchorLift,
		string(types.MetricAnchorLift), dimensionless)
	putFrameMetric(measurement, output, algo.SymbolLadderSpreadTightening,
		string(types.MetricSpreadTightening), dimensionless)
	putFrameMetric(measurement, output, algo.SymbolLadderSpreadDeviation,
		string(types.MetricSpreadDeviation), dimensionless)
	putFrameMetric(measurement, output, algo.SymbolLadderBidDepth,
		string(types.MetricLadderBidDepth), baseQuantity)
	putFrameMetric(measurement, output, algo.SymbolLadderAskDepth,
		string(types.MetricLadderAskDepth), baseQuantity)
	putFrameMetric(measurement, output, algo.SymbolLadderImbalance,
		string(types.MetricLadderImbalance), dimensionless)
	putFrameMetric(measurement, output, algo.SymbolLadderSpreadBaseline,
		string(types.MetricLadderSpreadBaseline), price)
	putNormalizedFrameMetric(measurement, output, algo.SymbolCompression,
		algo.SymbolCompression, string(types.MetricCompression), dimensionless, ladderReady)
	putFrameMetric(measurement, output, algo.SymbolLadderBidDepletion,
		string(types.MetricLadderBidDepletion), dimensionless)
	putFrameMetric(measurement, output, algo.SymbolLadderAskDepletion,
		string(types.MetricLadderAskDepletion), dimensionless)
	putFrameMetric(measurement, output, algo.SymbolLadderBidReplenish,
		string(types.MetricLadderBidReplenish), dimensionless)
	putFrameMetric(measurement, output, algo.SymbolLadderAskReplenish,
		string(types.MetricLadderAskReplenish), dimensionless)
	putNormalizedFrameMetric(measurement, output, algo.SymbolAlphaPrecursor,
		algo.SymbolAlphaPrecursorNormalized,
		types.MetricKey(types.MetricPrecursor, types.SideBuy), dimensionless, anchorReady)
	putNormalizedFrameMetric(measurement, output, algo.SymbolBetaPrecursor,
		algo.SymbolBetaPrecursorNormalized,
		types.MetricKey(types.MetricPrecursor, types.SideSell), dimensionless, anchorReady)
	putNormalizedFrameMetric(measurement, output, algo.SymbolAlphaExhaustion,
		algo.SymbolAlphaExhaustion,
		types.MetricKey(types.MetricExhaustion, types.SideBuy), dimensionless, ignitionReady)
	putNormalizedFrameMetric(measurement, output, algo.SymbolBetaExhaustion,
		algo.SymbolBetaExhaustion,
		types.MetricKey(types.MetricExhaustion, types.SideSell), dimensionless, ignitionReady)
	putNormalizedFrameMetric(measurement, output,
		algo.SymbolIgnitionHypothesisSeparation,
		algo.SymbolIgnitionHypothesisSeparation,
		string(types.MetricHypothesisSeparation), dimensionless, ignitionReady)

	return measurement
}

func putFrameMetric(
	measurement *nmtypes.Measurement,
	output nomagique.Frame,
	symbol nomagique.Symbol,
	name string,
	descriptor nmtypes.Descriptor,
) {
	raw, found := output.Get(symbol)

	if !found {
		return
	}

	measurement.Put(name, nmtypes.NewMetric(name, raw, descriptor))
}

func putNormalizedFrameMetric(
	measurement *nmtypes.Measurement,
	output nomagique.Frame,
	rawSymbol nomagique.Symbol,
	normalizedSymbol nomagique.Symbol,
	name string,
	descriptor nmtypes.Descriptor,
	ready bool,
) {
	raw, found := output.Get(rawSymbol)

	if !found {
		return
	}

	metric := nmtypes.NewMetric(name, raw, descriptor)
	normalized, hasNormalized := output.Get(normalizedSymbol)

	if ready && hasNormalized {
		metric = nmtypes.NewNormalizedMetric(name, raw, normalized, descriptor)
	}

	measurement.Put(name, metric)
}

func putMetadata(
	measurement *nmtypes.Measurement,
	output nomagique.Frame,
	name string,
	symbol nomagique.Symbol,
) {
	value, found := output.Get(symbol)

	if found {
		measurement.Metadata[name] = value
	}
}

func pumpDumpObservedFrom(output nomagique.Frame, fallback time.Time) time.Time {
	seconds, hasSeconds := output.Get(algo.SymbolPumpDumpObservedFromSec)
	nanoseconds, hasNanoseconds := output.Get(algo.SymbolPumpDumpObservedFromNsec)

	if !hasSeconds || !hasNanoseconds || seconds == 0 && nanoseconds == 0 {
		return fallback
	}

	return time.Unix(int64(seconds), int64(nanoseconds)).UTC()
}

func frameNumber(frame nomagique.Frame, symbol nomagique.Symbol) float64 {
	value, _ := frame.Get(symbol)

	return value
}
