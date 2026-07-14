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
	return []*types.Measurement{
		types.ObservationMeasurement(
			types.SourceDepthFlow, types.DepthFlow, types.MetricLoadedScore,
			types.SubjectBookImbalance, symbol, at,
			types.UnitDimensionless, output.LoadedScore, maturity,
		),
		types.ObservationMeasurement(
			types.SourceDepthFlow, types.DepthFlow, types.MetricSpoofScore,
			types.SubjectBookImbalance, symbol, at,
			types.UnitDimensionless, output.SpoofScore, maturity,
		),
		types.ObservationMeasurement(
			types.SourceDepthFlow, types.DepthFlow, types.MetricThinScore,
			types.SubjectBookImbalance, symbol, at,
			types.UnitDimensionless, output.ThinScore, maturity,
		),
		types.ObservationMeasurement(
			types.SourceDepthFlow, types.DepthFlow, types.MetricNeutralScore,
			types.SubjectBookImbalance, symbol, at,
			types.UnitDimensionless, output.NeutralScore, maturity,
		),
		types.ObservationMeasurement(
			types.SourceDepthFlow, types.DepthFlow, types.MetricStrength,
			types.SubjectBookImbalance, symbol, at,
			types.UnitDimensionless, output.Strength, maturity,
		),
		types.ObservationMeasurement(
			types.SourceDepthFlow, types.DepthFlow, types.MetricValue,
			types.SubjectBookImbalance, symbol, at,
			types.UnitDimensionless, output.Value, maturity,
		),
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
