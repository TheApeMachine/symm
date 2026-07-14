package depthflow

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/types"
)

/*
DepthFlow is the "Weight of the Book" perspective, measuring touch-level book
imbalance with trade-pressure confirmation. Categories belong in logic; this
signal emits numerical scores only.
*/
type Signal struct {
	ctx      context.Context
	cancel   context.CancelFunc
	book     *Book
	trade    *Trade
	sample   *flow.Sample
	bookflow *equation.Bookflow
}

func NewSignal(
	ctx context.Context, api *websocket.API, instrument *trader.Instrument,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:      ctx,
		cancel:   cancel,
		book:     NewBook(ctx, api, instrument),
		trade:    NewTrade(ctx, api),
		sample:   flow.NewSample(),
		bookflow: equation.NewBookflow(),
	}
}

/*
Measure converts the receiver's current market input into typed measurements
so downstream logic consumes explicit evidence.
*/
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

		output, err := signal.bookflow.Measure(input)

		if err != nil {
			panic(errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				err.Error(),
				err,
			)))
		}

		if !output.Ready {
			continue
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

		output, err := signal.bookflow.Measure(input)

		if err != nil {
			panic(errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				err.Error(),
				err,
			)))
		}

		if !output.Ready {
			continue
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
measurements converts a bookflow calculator output into the shared
Measurement shape, so both the book-driven and trade-driven observation paths
emit the same metric set for a symbol.
*/
func (signal *Signal) measurements(
	symbol string, at time.Time, output equation.BookflowOutput, maturity float64,
) []*types.Measurement {
	validity := types.MeasurementValidity{
		State:     types.ValidityValid,
		Readiness: types.ReadinessObservation,
	}
	scale := types.ScaleReference{
		Kind:    types.ScaleObservationWindow,
		From:    at,
		Through: at,
	}
	specs := []struct {
		metric types.MetricType
		raw    float64
	}{
		{types.MetricLoadedScore, output.LoadedScore},
		{types.MetricSpoofScore, output.SpoofScore},
		{types.MetricThinScore, output.ThinScore},
		{types.MetricNeutralScore, output.NeutralScore},
		{types.MetricStrength, output.Strength},
		{types.MetricValue, output.Value},
	}
	measurements := make([]*types.Measurement, 0, len(specs))

	for _, spec := range specs {
		measurements = append(measurements, &types.Measurement{
			Source:     types.SourceDepthFlow,
			Stream:     types.DepthFlow,
			Metric:     spec.metric,
			Subject:    types.SubjectBookImbalance,
			Symbol:     symbol,
			At:         at,
			Unit:       types.UnitDimensionless,
			Raw:        spec.raw,
			Normalized: types.NormalizeFinite(spec.raw),
			Maturity:   maturity,
			Validity:   validity,
			Scale:      scale,
		})
	}

	return measurements
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() (err error) {
	err = errnie.Error(errnie.Err(
		errnie.Internal,
		"signal: close failed",
		nil,
	))

	signal.cancel()
	return err
}
