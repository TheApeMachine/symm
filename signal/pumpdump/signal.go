package pumpdump

import (
	"context"

	"github.com/google/uuid"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Signal owns pump-cycle measurements derived from executed trade lift and the
reconstructed book's midpoint and spread. It reads each market
fact from its authoritative stream without treating them as independent
corroborating signals.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	api    *websocket.API
	ui     chan []byte
	algo   *equation.Ignition
	quotes *types.QuoteHistory
}

/*
NewSignal creates an empty per-symbol pump state whose baseline capacity is the
same explicit retention bound used by the production market feed.
*/
func NewSignal(
	ctx context.Context,
	api *websocket.API,
	ui chan []byte,
) *Signal {
	return NewSignalWithQuotes(
		ctx,
		api,
		ui,
		types.NewQuoteHistory(system.Cfg.PumpDump.Capacity),
	)
}

/*
NewSignalWithQuotes creates ignition state sharing the owning tape shard's
causal quote history.
*/
func NewSignalWithQuotes(
	ctx context.Context,
	api *websocket.API,
	ui chan []byte,
	quotes *types.QuoteHistory,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:    ctx,
		cancel: cancel,
		api:    api,
		ui:     ui,
		algo: equation.NewIgnition(
			system.Cfg.PumpDump.Capacity,
		),
		quotes: quotes,
	}

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourcePumpDump)
}

func (signal *Signal) Type() types.SourceType {
	return types.SourcePumpDump
}

/*
Measure produces the Measurements for the pumpdump signal.
*/
func (signal *Signal) Measure(symbol *types.Symbol, _ ...int64) []*types.Measurement {
	utils.PublishPriority(signal.ui, datura.NewMap("activity", datura.NewMap(
		string(types.SourcePumpDump), "running",
	)))

	defer utils.PublishPriority(signal.ui, datura.NewMap("activity", datura.NewMap(
		string(types.SourcePumpDump), "done",
	)))

	measurements := make([]*types.Measurement, 0)
	signal.ingestQuotes(symbol)

	for trade := range symbol.MarketTrades(types.SourcePumpDump) {
		bid, ask, found := signal.quote(trade)

		if !found {
			continue
		}

		output, ready, maturity, err := signal.algo.Measure(equation.IgnitionInput{
			Symbol: symbol.Symbol,
			Volume: trade.Qty,
			Last:   trade.Price.Float64(),
			Bid:    bid,
			Ask:    ask,
			At:     trade.Timestamp,
		})

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"pumpdump: failed to measure ignition",
				err,
			))

			continue
		}

		measurement := &types.Measurement{
			ID:       uuid.NewString(),
			Source:   types.SourcePumpDump,
			Symbol:   trade.Symbol,
			Tick:     symbol.Tick,
			At:       trade.Timestamp,
			Maturity: maturity,
			Metadata: map[string]float64{
				"ask":            ask,
				"bid":            bid,
				"trade_price":    trade.Price.Float64(),
				"trade_quantity": trade.Qty,
			},
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricBestPrice, types.SideBuy): {
					Raw:  bid,
					Unit: types.UnitQuoteCurrency,
				},
				types.MetricKey(types.MetricBestPrice, types.SideSell): {
					Raw:  ask,
					Unit: types.UnitQuoteCurrency,
				},
				types.MetricKey(types.MetricMidpoint, types.SideNone): {
					Raw:  (bid + ask) / 2,
					Unit: types.UnitQuoteCurrency,
				},
				types.MetricKey(types.MetricTradePrice, types.SideNone): {
					Raw:  trade.Price.Float64(),
					Unit: types.UnitQuoteCurrency,
				},
				types.MetricKey(types.MetricTradeQuantity, types.SideNone): {
					Raw:  trade.Qty,
					Unit: types.UnitBaseCurrency,
				},
				types.MetricKey(types.MetricRVOL, types.SideNone): {
					Raw:        output.RVOL,
					Normalized: normalizedIgnitionEvidence(types.MetricRVOL, output.RVOL, ready),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricPrecursor, types.SideNone): {
					Raw:        output.Precursor,
					Normalized: normalizedIgnitionEvidence(types.MetricPrecursor, output.Precursor, ready),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricSpread, types.SideNone): {
					Raw:        output.Spread,
					Normalized: normalizedSpread(output.Spread, (bid+ask)/2),
					Unit:       types.UnitQuoteCurrency,
				},
				types.MetricKey(types.MetricCompression, types.SideNone): {
					Raw:        output.Compression,
					Normalized: normalizedIgnitionEvidence(types.MetricCompression, output.Compression, ready),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricIgnition, types.SideNone): {
					Raw:        output.Ignition,
					Normalized: normalizedIgnitionEvidence(types.MetricIgnition, output.Ignition, ready),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricTrend, types.SideNone): {
					Raw:        output.Trend,
					Normalized: normalizedIgnitionEvidence(types.MetricTrend, output.Trend, ready),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricExhaustion, types.SideNone): {
					Raw:        output.Exhaustion,
					Normalized: normalizedIgnitionEvidence(types.MetricExhaustion, output.Exhaustion, ready),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricStrength, types.SideNone): {
					Raw:        output.Strength,
					Normalized: normalizedIgnitionEvidence(types.MetricStrength, output.Strength, ready),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricPrecursor, types.SideBuy): {
					Raw:        output.Buy.Precursor,
					Normalized: normalizedIgnitionEvidence(types.MetricPrecursor, output.Buy.Precursor, ready),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricCompression, types.SideBuy): {
					Raw:        output.Buy.Compression,
					Normalized: normalizedIgnitionEvidence(types.MetricCompression, output.Buy.Compression, ready),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricIgnition, types.SideBuy): {
					Raw:        output.Buy.Ignition,
					Normalized: normalizedIgnitionEvidence(types.MetricIgnition, output.Buy.Ignition, ready),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricTrend, types.SideBuy): {
					Raw:        output.Buy.Trend,
					Normalized: normalizedIgnitionEvidence(types.MetricTrend, output.Buy.Trend, ready),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricExhaustion, types.SideBuy): {
					Raw:        output.Buy.Exhaustion,
					Normalized: normalizedIgnitionEvidence(types.MetricExhaustion, output.Buy.Exhaustion, ready),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricStrength, types.SideBuy): {
					Raw:        output.Buy.Strength,
					Normalized: normalizedIgnitionEvidence(types.MetricStrength, output.Buy.Strength, ready),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricPrecursor, types.SideSell): {
					Raw:        output.Sell.Precursor,
					Normalized: normalizedIgnitionEvidence(types.MetricPrecursor, output.Sell.Precursor, ready),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricCompression, types.SideSell): {
					Raw:        output.Sell.Compression,
					Normalized: normalizedIgnitionEvidence(types.MetricCompression, output.Sell.Compression, ready),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricIgnition, types.SideSell): {
					Raw:        output.Sell.Ignition,
					Normalized: normalizedIgnitionEvidence(types.MetricIgnition, output.Sell.Ignition, ready),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricTrend, types.SideSell): {
					Raw:        output.Sell.Trend,
					Normalized: normalizedIgnitionEvidence(types.MetricTrend, output.Sell.Trend, ready),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricExhaustion, types.SideSell): {
					Raw:        output.Sell.Exhaustion,
					Normalized: normalizedIgnitionEvidence(types.MetricExhaustion, output.Sell.Exhaustion, ready),
					Unit:       types.UnitDimensionless,
				},
				types.MetricKey(types.MetricStrength, types.SideSell): {
					Raw:        output.Sell.Strength,
					Normalized: normalizedIgnitionEvidence(types.MetricStrength, output.Sell.Strength, ready),
					Unit:       types.UnitDimensionless,
				},
			},
		}

		separation, separationReady := types.MeasurementHypothesisSeparation(
			types.SourcePumpDump,
			measurement.Metrics,
		)

		snrSample := types.MetricSample{
			Raw:  separation,
			Unit: types.UnitDimensionless,
		}

		if separationReady {
			snrSample.Normalized = &separation
		}

		measurement.PutMetric(types.MetricHypothesisSeparation, types.SideNone, snrSample)
		measurements = append(measurements, measurement)
	}

	return measurements
}

func (signal *Signal) ingestQuotes(symbol *types.Symbol) {
	for ticker := range symbol.MarketTickers(types.SourcePumpDump) {
		signal.quotes.Observe(ticker)
	}
}

func (signal *Signal) quote(trade kraken.TradeData) (float64, float64, bool) {
	if signal.quotes == nil {
		return 0, 0, false
	}

	ticker, found := signal.quotes.At(trade.Symbol, trade.Timestamp)

	if !found {
		return 0, 0, false
	}

	return ticker.Bid.Float64(), ticker.Ask.Float64(), true
}

/*
normalizedIgnitionEvidence accepts only ready empirical ignition evidence.
Unbounded baseline ratios use parity as their domain scale and map to their
share against parity; bounded scores retain their calculated value. Before the
volume-clock baselines mature, raw placeholders cannot enter normalized math.
*/
func normalizedIgnitionEvidence(
	metric types.MetricType,
	raw float64,
	ready bool,
) *float64 {
	if !ready {
		return nil
	}

	if raw < 0 {
		return nil
	}

	value := raw

	if metric == types.MetricRVOL || metric == types.MetricPrecursor ||
		metric == types.MetricIgnition || metric == types.MetricTrend ||
		metric == types.MetricStrength {
		value = raw / (1 + raw)
	} else if raw > 1 {
		return nil
	}

	return &value
}

/*
normalizedSpread reports executable spread as a fraction of the authoritative
book midpoint observed no later than the trade.
*/
func normalizedSpread(raw, midpoint float64) *float64 {
	if raw <= 0 || midpoint <= 0 {
		return nil
	}

	value := raw / midpoint

	return &value
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	if signal.cancel != nil {
		signal.cancel()
	}

	return nil
}
