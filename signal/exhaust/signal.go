package exhaust

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/types"
)

/*
Exhaust is the Exit Thesis perspective, tracking microstructure decay to advise
on the urgency of closing an open position. Categories belong in logic; this
signal emits numerical scores only.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	book   *Book
	trade  *Trade
	sample *algorithm.DecaySample
	decay  *equation.Decay
}

func NewSignal(
	ctx context.Context, api *websocket.API, instrument *trader.Instrument,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		book:   NewBook(ctx, api, instrument),
		trade:  NewTrade(ctx, api),
		sample: algorithm.NewDecaySample(),
		decay:  equation.NewDecay(),
	}
}

func (signal *Signal) Measure(
	thesis *types.Thesis,
) *types.Thesis {
	books := signal.book.cache
	trades := signal.trade.cache
	out := make([]*types.Measurement, 0, len(books)+len(trades))

	for _, row := range books {
		if row.Symbol == "" || row.PriceIncrement.Sign() <= 0 {
			continue
		}

		bids := make([]flow.BookLevel, 0, len(row.Bids))
		asks := make([]flow.BookLevel, 0, len(row.Asks))

		for _, level := range row.Bids {
			tick, err := kraken.PriceTick(level.Price, row.PriceIncrement)

			if err != nil {
				panic(errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					err.Error(),
					err,
				)))
			}

			bids = append(bids, flow.BookLevel{
				Price:    level.Price.Float64(),
				Ticks:    tick,
				Quantity: level.Qty,
			})
		}

		for _, level := range row.Asks {
			tick, err := kraken.PriceTick(level.Price, row.PriceIncrement)

			if err != nil {
				panic(errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					err.Error(),
					err,
				)))
			}

			asks = append(asks, flow.BookLevel{
				Price:    level.Price.Float64(),
				Ticks:    tick,
				Quantity: level.Qty,
			})
		}

		input, ready, maturity, err := signal.sample.MeasureBook(flow.BookInput{
			Symbol:   row.Symbol,
			TickSize: row.PriceIncrement.Float64(),
			Bids:     bids,
			Asks:     asks,
		})

		if err != nil {
			panic(errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				err.Error(),
				err,
			)))
		}

		if !ready {
			continue
		}

		output, err := signal.decay.Measure(input)

		if err != nil {
			panic(errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				err.Error(),
				err,
			)))
		}

		out = append(out, signal.measurements(row.Symbol, row.Timestamp, output, maturity)...)
	}

	for _, row := range trades {
		if row.Symbol == "" || row.Price.Sign() <= 0 || row.Qty <= 0 {
			continue
		}

		input, ready, maturity, err := signal.sample.MeasureTrade(flow.TradeInput{
			Symbol:   row.Symbol,
			Price:    row.Price.Float64(),
			Quantity: row.Qty,
			Side:     row.Side,
		})

		if err != nil {
			panic(errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				err.Error(),
				err,
			)))
		}

		if !ready {
			continue
		}

		output, err := signal.decay.Measure(input)

		if err != nil {
			panic(errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				err.Error(),
				err,
			)))
		}

		out = append(out, signal.measurements(row.Symbol, row.Timestamp, output, maturity)...)
	}

	signal.book.cache = signal.book.cache[:0]
	signal.trade.cache = signal.trade.cache[:0]

	thesis.Signals.Store("books", books)
	thesis.Signals.Store("trades", trades)
	thesis.Measurements = append(thesis.Measurements, out...)

	return thesis
}

/*
measurements converts a decay calculator output into the shared Measurement
shape, so both the book-driven and trade-driven observation paths emit the
same metric set for a symbol.
*/
func (signal *Signal) measurements(
	symbol string, at time.Time, output equation.DecayOutput, maturity float64,
) []*types.Measurement {
	validity := types.MeasurementValidity{
		State:     types.ValidityValid,
		Readiness: types.ReadinessObservation,
	}

	return []*types.Measurement{
		{Source: types.SourceExhaustion, Metric: types.MetricMechanical, Stream: types.Exhaust, Symbol: symbol, At: at, Unit: types.UnitDimensionless, Raw: output.Mechanical, Maturity: maturity, Validity: validity},
		{Source: types.SourceExhaustion, Metric: types.MetricThermal, Stream: types.Exhaust, Symbol: symbol, At: at, Unit: types.UnitDimensionless, Raw: output.Thermal, Maturity: maturity, Validity: validity},
		{Source: types.SourceExhaustion, Metric: types.MetricFragile, Stream: types.Exhaust, Symbol: symbol, At: at, Unit: types.UnitDimensionless, Raw: output.Fragile, Maturity: maturity, Validity: validity},
		{Source: types.SourceExhaustion, Metric: types.MetricReversal, Stream: types.Exhaust, Symbol: symbol, At: at, Unit: types.UnitDimensionless, Raw: output.Reversal, Maturity: maturity, Validity: validity},
		{Source: types.SourceExhaustion, Metric: types.MetricUrgency, Stream: types.Exhaust, Symbol: symbol, At: at, Unit: types.UnitDimensionless, Raw: output.Urgency, Maturity: maturity, Validity: validity},
		{Source: types.SourceExhaustion, Metric: types.MetricStrength, Stream: types.Exhaust, Symbol: symbol, At: at, Unit: types.UnitDimensionless, Raw: output.Strength, Maturity: maturity, Validity: validity},
		{Source: types.SourceExhaustion, Metric: types.MetricValue, Stream: types.Exhaust, Symbol: symbol, At: at, Unit: types.UnitDimensionless, Raw: output.Value, Maturity: maturity, Validity: validity},
		{Source: types.SourceExhaustion, Metric: types.MetricCategory, Stream: types.Exhaust, Symbol: symbol, At: at, Unit: types.UnitDimensionless, Raw: output.Category, Maturity: maturity, Validity: validity},
	}
}

func (signal *Signal) Close() (err error) {
	err = errnie.Error(errnie.Err(
		errnie.Internal,
		"signal: close failed",
		nil,
	))

	signal.cancel()
	return err
}
