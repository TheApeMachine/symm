package pumpdump

import (
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/statistic"
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
)

func (signal *Signal) bookMeasurement(
	at time.Time,
	geometry nomagique.Frame,
	alphaChange nomagique.Frame,
	betaChange nomagique.Frame,
) *nmtypes.Measurement {
	separation := 0.0
	baseline, hasBaseline := geometry.Get(statistic.SymbolMean)
	_, hasCompression := geometry.Get(equation.SymbolCompression)

	if hasBaseline && hasCompression {
		comparison := nomagique.Frame{}
		comparison.Put(nmtypes.AlphaQuantity, geometry.MustGet(equation.SymbolWidth))
		comparison.Put(nmtypes.BetaQuantity, baseline)
		_, comparison, err := nomagique.Step(
			signal.separate,
			nomagique.Frame{},
			comparison,
		)

		if err != nil {
			panic(err)
		}

		separation = comparison.MustGet(statistic.SymbolSeparation)
	}

	measurement := signal.newMeasurement(
		at,
		at,
		maturity(geometry),
		separation,
	)
	alphaPrice := geometry.MustGet(nmtypes.AlphaPrice)
	betaPrice := geometry.MustGet(nmtypes.BetaPrice)
	alphaQuantity := geometry.MustGet(nmtypes.AlphaQuantity)
	betaQuantity := geometry.MustGet(nmtypes.BetaQuantity)
	measurement.Metadata["alpha_price"] = alphaPrice
	measurement.Metadata["beta_price"] = betaPrice
	measurement.Metadata["alpha_quantity"] = alphaQuantity
	measurement.Metadata["beta_quantity"] = betaQuantity
	measurement.Metadata["relative_spread"] = geometry.MustGet(
		equation.SymbolRelativeWidth,
	)
	measurement.AddMetrics(
		rawMetric(types.MetricKey(types.MetricBestPrice, types.SideBuy), alphaPrice, price),
		rawMetric(types.MetricKey(types.MetricBestPrice, types.SideSell), betaPrice, price),
		rawMetric(string(types.MetricMidpoint), geometry.MustGet(equation.SymbolCenter), price),
		normalizedMetric(
			string(types.MetricSpread),
			geometry.MustGet(equation.SymbolWidth),
			geometry.MustGet(equation.SymbolDissimilarity),
			price,
		),
		rawMetric(string(types.MetricLadderBidDepth), alphaQuantity, baseQuantity),
		rawMetric(string(types.MetricLadderAskDepth), betaQuantity, baseQuantity),
		rawMetric(
			string(types.MetricLadderImbalance),
			geometry.MustGet(equation.SymbolBalance),
			dimensionless,
		),
	)
	putOptionalRaw(
		measurement,
		geometry,
		statistic.SymbolMean,
		string(types.MetricLadderSpreadBaseline),
		price,
	)
	putOptionalNormalized(
		measurement,
		geometry,
		equation.SymbolCompression,
		string(types.MetricCompression),
		dimensionless,
	)
	putOptionalRaw(
		measurement,
		geometry,
		equation.SymbolCompression,
		string(types.MetricSpreadTightening),
		dimensionless,
	)
	putOptionalRaw(
		measurement,
		geometry,
		equation.SymbolDeviation,
		string(types.MetricSpreadDeviation),
		price,
	)
	signal.putDepthDynamics(measurement, alphaChange, betaChange)

	return measurement
}

func (signal *Signal) putDepthDynamics(
	measurement *nmtypes.Measurement,
	alphaChange nomagique.Frame,
	betaChange nomagique.Frame,
) {
	alpha, err := relativeComponents(signal.decompose, alphaChange)

	if err != nil {
		panic(err)
	}

	beta, err := relativeComponents(signal.decompose, betaChange)

	if err != nil {
		panic(err)
	}

	putOptionalRaw(measurement, alpha, equation.SymbolBeta,
		string(types.MetricLadderBidDepletion), dimensionless)
	putOptionalRaw(measurement, beta, equation.SymbolBeta,
		string(types.MetricLadderAskDepletion), dimensionless)
	putOptionalRaw(measurement, alpha, equation.SymbolAlpha,
		string(types.MetricLadderBidReplenish), dimensionless)
	putOptionalRaw(measurement, beta, equation.SymbolAlpha,
		string(types.MetricLadderAskReplenish), dimensionless)
}

func (signal *Signal) tickerMeasurement(
	ticker kraken.TickerData,
	displacement nomagique.Frame,
	magnitude nomagique.Frame,
	normalized nomagique.Frame,
	polarized nomagique.Frame,
) *nmtypes.Measurement {
	measurement := signal.newMeasurement(
		ticker.Timestamp,
		ticker.Timestamp,
		maturity(normalized),
		0,
	)
	measurement.Metadata["last"] = ticker.Last.Float64()
	measurement.Metadata["reported_volume"] = ticker.Volume

	if ticker.Vwap > 0 {
		measurement.Metadata["volume_weighted_reference"] = ticker.Vwap
	}

	if _, found := displacement.Get(equation.SymbolChange); !found {
		return measurement
	}

	putEvidence(
		measurement,
		string(types.MetricAnchorDetach),
		magnitude.MustGet(nomagique.SampleValue),
		normalized,
	)
	putOptionalRaw(measurement, normalized, equation.SymbolLift,
		string(types.MetricAnchorLift), dimensionless)
	putPolarized(measurement, polarized)

	return measurement
}

func (signal *Signal) tradeMeasurement(
	trade kraken.TradeData,
	acceleration nomagique.Frame,
	rate nomagique.Frame,
	change nomagique.Frame,
	polarized nomagique.Frame,
	exhaustion nomagique.Frame,
) *nmtypes.Measurement {
	from := observedFrom(acceleration, trade.Timestamp)
	separation := frameNumber(exhaustion, statistic.SymbolSeparation)
	measurement := signal.newMeasurement(
		trade.Timestamp,
		from,
		maturity(acceleration),
		separation,
	)
	measurement.Metadata["trade_price"] = trade.Price.Float64()
	measurement.Metadata["trade_quantity"] = trade.Qty
	putMetadata(measurement, acceleration, "quantity_target", equation.SymbolTarget)
	putMetadata(measurement, acceleration, "completed_rate", calculus.SymbolRate)
	putMetadata(measurement, acceleration, "completed_change", equation.SymbolChange)
	putMetadata(measurement, rate, "rate_baseline", statistic.SymbolMean)
	putMetadata(measurement, change, "change_baseline", statistic.SymbolMean)
	measurement.AddMetrics(
		rawMetric(string(types.MetricTradePrice), trade.Price.Float64(), price),
		rawMetric(string(types.MetricTradeQuantity), trade.Qty, baseQuantity),
	)
	putEvidence(measurement, string(types.MetricRVOL),
		frameNumber(rate, equation.SymbolRatio), rate)
	putOptionalRaw(measurement, rate, equation.SymbolLift,
		string(types.MetricRVOLLift), dimensionless)
	putPolarized(measurement, polarized)
	putExhaustion(measurement, exhaustion)

	return measurement
}

func (signal *Signal) newMeasurement(
	at time.Time,
	from time.Time,
	maturityValue float64,
	separation float64,
) *nmtypes.Measurement {
	measurement := nmtypes.NewMeasurement(
		uuid.NewString(),
		signal.Name(),
		at.UnixNano(),
		from.UnixNano(),
	)
	measurement.At = at
	measurement.ObservedFrom = from
	measurement.Horizon = at.Sub(from)
	measurement.Maturity = maturityValue
	measurement.Metadata = make(map[string]float64)
	measurement.Put(
		string(types.MetricHypothesisSeparation),
		normalizedMetric(
			string(types.MetricHypothesisSeparation),
			separation,
			separation,
			dimensionless,
		),
	)

	return measurement
}

func putPolarized(
	measurement *nmtypes.Measurement,
	polarized nomagique.Frame,
) {
	putDirectionalEvidence(measurement, polarized,
		equation.SymbolAlpha, equation.SymbolAlphaNormalized, types.SideBuy)
	putDirectionalEvidence(measurement, polarized,
		equation.SymbolBeta, equation.SymbolBetaNormalized, types.SideSell)
}

func putDirectionalEvidence(
	measurement *nmtypes.Measurement,
	frame nomagique.Frame,
	rawSymbol nomagique.Symbol,
	normalizedSymbol nomagique.Symbol,
	side types.MeasurementSide,
) {
	raw, found := frame.Get(rawSymbol)

	if !found {
		return
	}

	name := types.MetricKey(types.MetricPrecursor, side)
	metric := rawMetric(name, raw, dimensionless)
	normalized, hasNormalized := frame.Get(normalizedSymbol)

	if hasNormalized {
		metric = normalizedMetric(name, raw, normalized, dimensionless)
	}

	measurement.Put(name, metric)
}

func putExhaustion(
	measurement *nmtypes.Measurement,
	exhaustion nomagique.Frame,
) {
	putOptionalNormalized(measurement, exhaustion, nmtypes.AlphaQuantity,
		types.MetricKey(types.MetricExhaustion, types.SideBuy), dimensionless)
	putOptionalNormalized(measurement, exhaustion, nmtypes.BetaQuantity,
		types.MetricKey(types.MetricExhaustion, types.SideSell), dimensionless)
}

func putEvidence(
	measurement *nmtypes.Measurement,
	name string,
	raw float64,
	normalized nomagique.Frame,
) {
	if _, found := normalized.Get(equation.SymbolRatio); !found {
		return
	}

	value, found := normalized.Get(equation.SymbolNormalized)

	if !found {
		measurement.Put(name, rawMetric(name, raw, dimensionless))
		return
	}

	measurement.Put(name, normalizedMetric(name, raw, value, dimensionless))
}

func putOptionalRaw(
	measurement *nmtypes.Measurement,
	frame nomagique.Frame,
	symbol nomagique.Symbol,
	name string,
	descriptor nmtypes.Descriptor,
) {
	value, found := frame.Get(symbol)

	if found {
		measurement.Put(name, rawMetric(name, value, descriptor))
	}
}

func putOptionalNormalized(
	measurement *nmtypes.Measurement,
	frame nomagique.Frame,
	symbol nomagique.Symbol,
	name string,
	descriptor nmtypes.Descriptor,
) {
	value, found := frame.Get(symbol)

	if found {
		measurement.Put(name, normalizedMetric(name, value, value, descriptor))
	}
}

func putMetadata(
	measurement *nmtypes.Measurement,
	frame nomagique.Frame,
	name string,
	symbol nomagique.Symbol,
) {
	value, found := frame.Get(symbol)

	if found {
		measurement.Metadata[name] = value
	}
}

func rawMetric(
	name string,
	value float64,
	descriptor nmtypes.Descriptor,
) *nmtypes.Metric[float64] {
	return nmtypes.NewMetric(name, value, descriptor)
}

func normalizedMetric(
	name string,
	raw float64,
	normalized float64,
	descriptor nmtypes.Descriptor,
) *nmtypes.Metric[float64] {
	return nmtypes.NewNormalizedMetric(name, raw, normalized, descriptor)
}

func maturity(frame nomagique.Frame) float64 {
	return frameNumber(frame, statistic.SymbolMaturity)
}

func frameNumber(frame nomagique.Frame, symbol nomagique.Symbol) float64 {
	value, _ := frame.Get(symbol)

	return value
}
