package toxicity

import (
	"context"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

const (
	touchSideNone = iota
	touchSideBid
	touchSideAsk
)

/*
Signal tracks whether near-touch liquidity is sincere, retreating, or bluffing
from Level3 order events corroborated by the public trade tape.
*/
type Signal struct {
	*types.Actor
	thesis   *types.Thesis
	ctx      context.Context
	cancel   context.CancelFunc
	level3   *websocket.API
	ui       chan []byte
	maturity int64
}

/*
NewSignal creates the Level3 honesty calculator against the production Kraken
API so tests can replace only its connections, never its market mechanics.
*/
func NewSignal(ctx context.Context, api *websocket.API, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:      ctx,
		cancel:   cancel,
		level3:   api,
		ui:       ui,
		maturity: 0,
	}

	signal.Actor = types.NewActor(ctx, "hawkes", map[string]types.Handler{
		"book": {
			Topic: "book",
			Fn:    signal.onBook,
		},
		"trade": {
			Topic: "trade",
			Fn:    signal.onTrade,
		},
	})

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceToxicity)
}

/*
Initialize wires book and trade ingress from Live.
*/
func (signal *Signal) Initialize(live *types.Actor, thesis *types.Thesis) {
	signal.thesis = thesis

	signal.Actor.Initialize(
		types.Topic{Name: "book", Actor: live},
		types.Topic{Name: "trade", Actor: live},
	)
}

func (signal *Signal) onBook(message any) any {
	rows := message.(*kraken.Book).Data

	signal.thesis.Measurements.Store(
		types.SourceToxicity, signal.Calculate(nil, nil, rows),
	)

	return signal.thesis
}

func (signal *Signal) onTrade(message any) any {
	rows := message.(*kraken.Trade).Data

	signal.thesis.Measurements.Store(
		types.SourceToxicity, signal.Calculate(nil, rows, nil),
	)

	return signal.thesis
}

func (signal *Signal) Calculate(
	tickers []kraken.TickerData,
	trades []kraken.TradeData,
	books []spot.BookManager,
) []*types.Measurement {
	signal.maturity = signal.thesis.Tick
	measurements := make([]*types.Measurement, 0)
	focusMeasurements := make([]*types.Measurement, 0)

	found, ok := signal.thesis.Measurements.Load(types.SourceToxicity)
	var priors []*types.Measurement
	var scaleFrom types.Measurement

	if ok {
		priors = found.([]*types.Measurement)

		if len(priors) > 0 && priors[len(priors)-1] != nil {
			scaleFrom = *priors[len(priors)-1]
		}
	}

	for _, trade := range trades {
		measurement := &types.Measurement{
			Source:   types.SourceToxicity,
			Symbol:   trade.Symbol,
			At:       trade.Timestamp.UTC(),
			Maturity: 1.0,
			Validity: types.ObservationValidity(len(priors) + 1),
			Scale: types.ScaleReference{
				Kind:    types.ScaleObservationWindow,
				From:    scaleFrom.At.UTC(),
				Through: trade.Timestamp.UTC(),
			},
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricTradeVolume, types.SideNone): {
					Raw:  trade.Price.Float64() * trade.Qty,
					Unit: types.UnitBaseCurrency,
				},
				types.MetricKey(types.MetricFillVolume, types.SideBuy): {
					Raw:  trade.Price.Float64() * trade.Qty,
					Unit: types.UnitQuoteCurrency,
				},
				types.MetricKey(types.MetricFillVolume, types.SideSell): {
					Raw:  trade.Price.Float64() * trade.Qty,
					Unit: types.UnitQuoteCurrency,
				},
			},
		}

		measurements = append(measurements, measurement)

		if trade.Symbol == types.Focus() {
			focusMeasurements = append(focusMeasurements, measurement)
		}
	}

	for _, book := range books {
		for _, ask := range book.Asks {
			bidQuantity := book.Book.BestBid().Quantity.Float64()
			touchQuantity := ask.Qty + bidQuantity
			quotedQuantity := decimal.ExactMul(
				decimal.NewFromFloat64(ask.Qty), &ask.Price,
			).Float64()

			measurement := &types.Measurement{
				Source:   types.SourceToxicity,
				Symbol:   book.Symbol,
				At:       book.Timestamp.UTC(),
				Maturity: float64(signal.maturity),
				Validity: types.ObservationValidity(len(priors) + 1),
				Scale: types.ScaleReference{
					Kind:    types.ScaleObservationWindow,
					From:    scaleFrom.At.UTC(),
					Through: book.Timestamp.UTC(),
				},
				Metrics: map[string]types.MetricSample{
					types.MetricKey(types.MetricTradeVolume, types.SideBuy): {
						Raw:  quotedQuantity,
						Unit: types.UnitBaseCurrency,
					},
					types.MetricKey(types.MetricFillVolume, types.SideBuy): {
						Raw:  quotedQuantity,
						Unit: types.UnitQuoteCurrency,
					},
					types.MetricKey(types.MetricAbsorption, types.SideBuy): {
						Raw:  quotedQuantity,
						Unit: types.UnitQuoteCurrency,
					},
					types.MetricKey(types.MetricBestPrice, types.SideBuy): {
						Raw:  book.Book.BestAsk().Price.Float64(),
						Unit: types.UnitQuoteCurrency,
					},
					types.MetricKey(types.MetricBestPrice, types.SideSell): {
						Raw:  book.Book.BestBid().Price.Float64(),
						Unit: types.UnitQuoteCurrency,
					},
					types.MetricKey(types.MetricTouchQuantity, types.SideBuy): {
						Raw: ask.Qty,
						Normalized: types.NormalizeRatio(
							ask.Qty, touchQuantity,
						),
						Unit: types.UnitBaseCurrency,
					},
					types.MetricKey(types.MetricTouchQuantity, types.SideSell): {
						Raw: bidQuantity,
						Normalized: types.NormalizeRatio(
							bidQuantity, touchQuantity,
						),
						Unit: types.UnitBaseCurrency,
					},
				},
			}

			measurements = append(measurements, measurement)

			if book.Symbol == types.Focus() {
				focusMeasurements = append(focusMeasurements, measurement)
			}
		}
	}

	if len(focusMeasurements) > 0 {
		utils.Publish(signal.ui, datura.NewMap("measurements", focusMeasurements))
	}

	return measurements
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
