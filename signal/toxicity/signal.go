package toxicity

import (
	"context"
	"iter"
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/algorithm/book/quality"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
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
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
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

func (signal *Signal) Measure(
	market *types.Symbol,
	ticks ...int64,
) iter.Seq[*types.Measurement] {
	return signal.measure(market, ticks...)
}

func (signal *Signal) measure(
	market *types.Symbol,
	ticks ...int64,
) iter.Seq[*types.Measurement] {
	return func(yield func(*types.Measurement) bool) {
		tick := market.Tick

		if len(ticks) > 0 {
			tick = ticks[0]
		}

		for trade := range market.MarketTrades(types.SourceToxicity) {
			_, _, _, err := signal.sample.MeasureTrade(flow.TradeInput{
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

		}

		for level3 := range market.MarketLevel3(types.SourceToxicity) {
			input, ready, maturity, err := signal.sample.MeasureLevel3(
				quality.Level3Input{
					Symbol: level3.Symbol,
					Bids:   qualityEvents(level3.Bids),
					Asks:   qualityEvents(level3.Asks),
				},
			)

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"toxicity: failed to sample Level 3 frame",
					err,
				))
				continue
			}

			at := level3Time(level3)

			if !ready || at.IsZero() {
				// The classifier only speaks once the quality sampler has a
				// complete frame. An immature frame is still an honest reading:
				// zero maturity, no scores, distinguishable from a wholly absent
				// pass — never a silent skip.
				if !yield(immatureToxicity(level3.Symbol, at, tick)) {
					return
				}

				continue
			}

			output, err := signal.bookQuality.Measure(input)

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"toxicity: failed to classify book quality",
					err,
				))
				continue
			}

			if !yield(&types.Measurement{
				ID:       uuid.NewString(),
				Source:   types.SourceToxicity,
				Symbol:   level3.Symbol,
				Tick:     tick,
				At:       at,
				Maturity: maturity,
				Metrics:  toxicityMetrics(input, output),
			}) {
				return
			}
		}
	}
}

func qualityEvents(orders []kraken.Level3Order) []quality.OrderEvent {
	events := make([]quality.OrderEvent, 0, len(orders))

	for _, order := range orders {
		if order.LimitPrice == nil || order.OrderQty == nil {
			continue
		}

		events = append(events, quality.OrderEvent{
			Event:    order.Event,
			OrderID:  order.OrderID,
			Price:    order.LimitPrice.Float64(),
			Quantity: order.OrderQty.Float64(),
		})
	}

	return events
}

func level3Time(level3 kraken.Level3Data) time.Time {
	at := level3.Timestamp

	for _, orders := range [][]kraken.Level3Order{level3.Bids, level3.Asks} {
		for _, order := range orders {
			if order.Timestamp.After(at) {
				at = order.Timestamp
			}
		}
	}

	return at
}

/*
immatureToxicity is the honest zero-reading for a dirty frame the quality
sampler cannot yet classify. It carries the symbol and event time so the pass
is observable without inventing scores.
*/
func immatureToxicity(symbol string, at time.Time, tick int64) *types.Measurement {
	return &types.Measurement{
		ID:       uuid.NewString(),
		Source:   types.SourceToxicity,
		Symbol:   symbol,
		Tick:     tick,
		At:       at,
		Maturity: 0,
		Metrics:  map[string]types.MetricSample{},
	}
}

func toxicityMetrics(
	input equation.BookQualityInput,
	output equation.BookQualityOutput,
) map[string]types.MetricSample {
	metrics := map[string]types.MetricSample{
		types.MetricKey(types.MetricCancelledQuantity, types.SideBuy): {
			Raw: input.CancelBid, Unit: types.UnitBaseCurrency,
		},
		types.MetricKey(types.MetricCancelledQuantity, types.SideSell): {
			Raw: input.CancelAsk, Unit: types.UnitBaseCurrency,
		},
		types.MetricKey(types.MetricFillVolume, types.SideBuy): {
			Raw: input.FillBid, Unit: types.UnitBaseCurrency,
		},
		types.MetricKey(types.MetricFillVolume, types.SideSell): {
			Raw: input.FillAsk, Unit: types.UnitBaseCurrency,
		},
		types.MetricKey(types.MetricTouchQuantity, types.SideBuy): {
			Raw: input.BidDepth, Unit: types.UnitBaseCurrency,
		},
		types.MetricKey(types.MetricTouchQuantity, types.SideSell): {
			Raw: input.AskDepth, Unit: types.UnitBaseCurrency,
		},
		types.MetricKey(types.MetricTradeVolume, types.SideNone): {
			Raw: input.FillBid + input.FillAsk, Unit: types.UnitBaseCurrency,
		},
		types.MetricKey(types.MetricMidpoint, types.SideNone): {
			Raw: output.Price, Unit: types.UnitQuoteCurrency,
		},
	}
	normalizeAttribution(metrics, input)

	for metric, value := range map[types.MetricType]float64{
		types.MetricBluffScore:   output.BluffScore,
		types.MetricVacuumScore:  output.VacuumScore,
		types.MetricSupportScore: output.SupportScore,
		types.MetricStrength:     output.Strength,
		types.MetricValue:        output.Value,
	} {
		normalized := value
		metrics[types.MetricKey(metric, types.SideNone)] = types.MetricSample{
			Raw: value, Normalized: &normalized, Unit: types.UnitDimensionless,
		}
	}

	metrics[types.MetricKey(types.MetricCategory, types.SideNone)] = types.MetricSample{
		Raw: output.Category, Unit: types.UnitDimensionless,
	}

	return metrics
}

/*
normalizeAttribution expresses the existing cancellation and fill quantities as
shares of the total accounted order-flow quantity. This preserves every raw
base-currency value while giving the competing evidence groups a common,
dimensionless denominator for HypothesisSeparation.
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

	separation, ready := types.MeasurementHypothesisSeparation(types.SourceToxicity, metrics)

	if !ready {
		return
	}

	metrics[types.MetricKey(types.MetricHypothesisSeparation, types.SideNone)] = types.MetricSample{
		Raw:        separation,
		Normalized: &separation,
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
