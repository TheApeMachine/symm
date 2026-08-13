package toxicity

import (
	"context"
	"time"

	"github.com/google/uuid"
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/algorithm/book/quality"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Signal tracks whether near-touch liquidity is sincere, retreating, or bluffing
from Level3 order events corroborated by the public trade tape.
*/
type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	books       websocket.BookSource
	ui          chan []byte
	sample      *quality.Sample
	bookQuality *equation.BookQuality
}

/*
NewSignal creates the Level3 honesty calculator against the production Kraken
API so tests can replace only its connections, never its market mechanics.
*/
func NewSignal(
	ctx context.Context,
	books websocket.BookSource,
	ui chan []byte,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		books:       books,
		ui:          ui,
		sample:      quality.NewSample(),
		bookQuality: equation.NewBookQuality(),
	}

	return signal
}

/*
Name returns the signal source identity.
*/
func (signal *Signal) Name() string {
	return string(types.SourceToxicity)
}

func (signal *Signal) Type() types.SourceType {
	return types.SourceToxicity
}

func (signal *Signal) Measure(market *types.Symbol) []*types.Measurement {
	measurements := make([]*types.Measurement, 0)

	for trade := range market.MarketTrades(types.SourceToxicity) {
		input, ready, maturity, err := signal.sample.MeasureTrade(flow.TradeInput{
			Symbol:   trade.Symbol,
			Price:    trade.Price.Float64(),
			Quantity: trade.Qty,
			Side:     flow.TradeSide(trade.Side),
			At:       trade.Timestamp,
		})

		if err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"toxicity: failed to sample trade",
				err,
			))

			continue
		}

		if !ready {
			continue
		}

		metrics := map[string]types.MetricSample{
			types.MetricKey(types.MetricCancelledQuantity, types.SideBuy): {
				Raw:  input.CancelBid,
				Unit: types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricCancelledQuantity, types.SideSell): {
				Raw:  input.CancelAsk,
				Unit: types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricRetreatingQuantity, types.SideBuy): {
				Raw:  input.BidDepth,
				Unit: types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricRetreatingQuantity, types.SideSell): {
				Raw:  input.AskDepth,
				Unit: types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricFillVolume, types.SideBuy): {
				Raw:  input.FillBid,
				Unit: types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricFillVolume, types.SideSell): {
				Raw:  input.FillAsk,
				Unit: types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricTradeVolume, types.SideNone): {
				Raw:  trade.Qty,
				Unit: types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricBestPrice, types.SideBuy): {
				Raw:  input.LastPrice,
				Unit: types.UnitQuoteCurrency,
			},
			types.MetricKey(types.MetricBestPrice, types.SideSell): {
				Raw:  input.LastPrice,
				Unit: types.UnitQuoteCurrency,
			},
			types.MetricKey(types.MetricTouchQuantity, types.SideBuy): {
				Raw:  input.BidDepth,
				Unit: types.UnitBaseCurrency,
			},
			types.MetricKey(types.MetricTouchQuantity, types.SideSell): {
				Raw:  input.AskDepth,
				Unit: types.UnitBaseCurrency,
			},
		}
		normalizeAttribution(metrics, input)

		measurement := &types.Measurement{
			ID:       uuid.NewString(),
			Source:   types.SourceToxicity,
			Symbol:   trade.Symbol,
			At:       trade.Timestamp,
			Maturity: maturity,
			Metrics:  metrics,
		}

		measurements = append(measurements, measurement)
	}

	var at time.Time
	var input equation.BookQualityInput
	var ready bool
	var maturity float64
	var err error

	signal.books.Book(market.Symbol, func(book *spotbook.Book) {
		bookInput := flow.BookInput{Symbol: market.Symbol}

		for level := book.Bids.Low; level != nil; level = level.Higher {
			bookInput.Bids = append(bookInput.Bids, flow.BookLevel{
				Price:    level.Price.Float64(),
				Quantity: level.Quantity.Float64(),
			})

			if level.Timestamp.After(at) {
				at = level.Timestamp
			}
		}

		for level := book.Asks.Low; level != nil; level = level.Higher {
			bookInput.Asks = append(bookInput.Asks, flow.BookLevel{
				Price:    level.Price.Float64(),
				Quantity: level.Quantity.Float64(),
			})

			if level.Timestamp.After(at) {
				at = level.Timestamp
			}
		}

		input, ready, maturity, err = signal.sample.MeasureBook(bookInput)
	})

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"toxicity: failed to sample book",
			err,
		))

		return measurements
	}

	if !ready {
		return measurements
	}

	if at.IsZero() {
		return measurements
	}

	metrics := map[string]types.MetricSample{
		types.MetricKey(types.MetricCancelledQuantity, types.SideBuy): {
			Raw:  input.CancelBid,
			Unit: types.UnitBaseCurrency,
		},
		types.MetricKey(types.MetricCancelledQuantity, types.SideSell): {
			Raw:  input.CancelAsk,
			Unit: types.UnitBaseCurrency,
		},
		types.MetricKey(types.MetricRetreatingQuantity, types.SideBuy): {
			Raw:  input.BidDepth,
			Unit: types.UnitBaseCurrency,
		},
		types.MetricKey(types.MetricRetreatingQuantity, types.SideSell): {
			Raw:  input.AskDepth,
			Unit: types.UnitBaseCurrency,
		},
		types.MetricKey(types.MetricFillVolume, types.SideBuy): {
			Raw:  input.FillBid,
			Unit: types.UnitBaseCurrency,
		},
		types.MetricKey(types.MetricFillVolume, types.SideSell): {
			Raw:  input.FillAsk,
			Unit: types.UnitBaseCurrency,
		},
		types.MetricKey(types.MetricTradeVolume, types.SideNone): {
			Raw:  input.FillBid + input.FillAsk,
			Unit: types.UnitBaseCurrency,
		},
		types.MetricKey(types.MetricBestPrice, types.SideBuy): {
			Raw:  input.LastPrice,
			Unit: types.UnitQuoteCurrency,
		},
		types.MetricKey(types.MetricBestPrice, types.SideSell): {
			Raw:  input.LastPrice,
			Unit: types.UnitQuoteCurrency,
		},
		types.MetricKey(types.MetricTouchQuantity, types.SideBuy): {
			Raw:  input.BidDepth,
			Unit: types.UnitBaseCurrency,
		},
		types.MetricKey(types.MetricTouchQuantity, types.SideSell): {
			Raw:  input.AskDepth,
			Unit: types.UnitBaseCurrency,
		},
	}
	normalizeAttribution(metrics, input)

	snr, ok := types.MeasurementSignalNoiseRatio(types.SourceToxicity, metrics)

	if ok {
		metrics[types.MetricKey(types.MetricSNR, types.SideNone)] = types.MetricSample{
			Raw:        snr,
			Normalized: &snr,
			Unit:       types.UnitDimensionless,
		}
	}

	measurement := &types.Measurement{
		ID:       uuid.NewString(),
		Source:   types.SourceToxicity,
		Symbol:   market.Symbol,
		At:       at,
		Maturity: maturity,
		Metrics:  metrics,
	}

	measurements = append(measurements, measurement)

	return measurements
}

/*
normalizeAttribution expresses the existing cancellation and fill quantities as
shares of the total accounted order-flow quantity. This preserves every raw
base-currency value while giving the competing evidence groups a common,
dimensionless denominator for SNR.
*/
func normalizeAttribution(
	metrics map[string]types.MetricSample,
	input equation.BookQualityInput,
) {
	total := input.CancelBid + input.CancelAsk + input.FillBid + input.FillAsk

	if total <= 0 {
		return
	}

	values := map[string]float64{
		types.MetricKey(types.MetricCancelledQuantity, types.SideBuy):  input.CancelBid / total,
		types.MetricKey(types.MetricCancelledQuantity, types.SideSell): input.CancelAsk / total,
		types.MetricKey(types.MetricFillVolume, types.SideBuy):         input.FillBid / total,
		types.MetricKey(types.MetricFillVolume, types.SideSell):        input.FillAsk / total,
	}

	for key, value := range values {
		sample := metrics[key]
		sample.Normalized = &value
		metrics[key] = sample
	}

	snr, ready := types.MeasurementSignalNoiseRatio(types.SourceToxicity, metrics)

	if !ready {
		return
	}

	metrics[types.MetricKey(types.MetricSNR, types.SideNone)] = types.MetricSample{
		Raw:        snr,
		Normalized: &snr,
		Unit:       types.UnitDimensionless,
	}
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
