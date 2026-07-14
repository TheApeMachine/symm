package toxicity

import (
	"context"
	"math"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Toxicity tracks whether near-touch liquidity is sincere, retreating, or bluffing
from level3 order events corroborated by the public trade tape.
*/
type Signal struct {
	ctx        context.Context
	cancel     context.CancelFunc
	trades     *Trade
	level3     *Level3
	priorTouch map[string]touchSnapshot
}

func NewSignal(ctx context.Context, api *websocket.API) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:        ctx,
		cancel:     cancel,
		trades:     NewTrade(ctx, api),
		level3:     NewLevel3(api),
		priorTouch: map[string]touchSnapshot{},
	}
}

/*
touchSnapshot retains prior best-level quantities so toxicity can distinguish
withdrawal from execution.
*/
type touchSnapshot struct {
	bidQuantity float64
	askQuantity float64
	observedAt  time.Time
}

/*
symbolEvidence aggregates trade and fill evidence for one symbol so touch
honesty uses one event batch.
*/
type symbolEvidence struct {
	latestAt    time.Time
	tradeCount  int
	volume      *decimal.Decimal
	fillBid     *decimal.Decimal
	fillAsk     *decimal.Decimal
	bidExecuted float64
	askExecuted float64
}

/*
observationContext carries the shared validity and scale contract for one
toxicity observation window anchored at a source event time.
*/
type observationContext struct {
	validity types.MeasurementValidity
	scale    types.ScaleReference
}

/*
newObservationContext builds validity from corroborating event count and scale
from the observation timestamp so Measure and touchHonesty share one contract.
*/
func newObservationContext(
	at time.Time,
	evidenceCount int,
) observationContext {
	return observationContext{
		validity: types.ObservationValidity(evidenceCount),
		scale: types.ScaleReference{
			Kind:    types.ScaleObservationWindow,
			From:    at,
			Through: at,
		},
	}
}

/*
Measure converts the receiver's current market input into typed measurements
so downstream logic consumes explicit evidence.
*/
func (signal *Signal) Measure(
	thesis *types.Thesis,
) *types.Thesis {
	trades := signal.trades.cache
	books := signal.level3.Books()
	evidence := map[string]*symbolEvidence{}

	for _, trade := range trades {
		if trade.Price == nil || trade.Volume == nil || trade.Price.Sign() <= 0 {
			continue
		}

		at, ok := tradeAt(trade)

		if !ok {
			continue
		}

		row := evidence[trade.Pair]

		if row == nil {
			row = &symbolEvidence{}
			evidence[trade.Pair] = row
		}

		row.tradeCount++
		row.volume = zeroed(row.volume).Add(trade.Volume)

		if at.After(row.latestAt) {
			row.latestAt = at
		}

		for bookManager := range books {
			book := bookManager.GetBook(trade.Pair)

			if book == nil || book.Name != trade.Pair {
				continue
			}

			bid, ask := book.BestBid(), book.BestAsk()

			if bid == nil || ask == nil {
				continue
			}

			if trade.Price.Cmp(bid.Price) == 0 {
				row.fillBid = zeroed(row.fillBid).Add(bid.Price.Mul(trade.Volume))
				row.bidExecuted += trade.Volume.Float64()
			}

			if trade.Price.Cmp(ask.Price) == 0 {
				row.fillAsk = zeroed(row.fillAsk).Add(ask.Price.Mul(trade.Volume))
				row.askExecuted += trade.Volume.Float64()
			}
		}
	}

	out := make([]*types.Measurement, 0, len(evidence)*8)
	nextTouch := map[string]touchSnapshot{}

	for symbol, row := range evidence {
		maturity := float64(row.tradeCount) / float64(row.tradeCount+1)
		obsCtx := newObservationContext(row.latestAt, row.tradeCount)

		out = append(out, &types.Measurement{
			Source:   types.SourceToxicity,
			Stream:   types.Toxicity,
			Metric:   types.MetricTradeVolume,
			Subject:  types.SubjectLevel3Tape,
			Symbol:   symbol,
			At:       row.latestAt,
			Unit:     types.UnitBaseCurrency,
			Raw:      row.volume.Float64(),
			Maturity: maturity,
			Validity: obsCtx.validity,
			Scale:    obsCtx.scale,
		})

		if row.fillBid != nil {
			out = append(out, &types.Measurement{
				Source:   types.SourceToxicity,
				Stream:   types.Toxicity,
				Metric:   types.MetricFillVolume,
				Subject:  types.SubjectLevel3Tape,
				Symbol:   symbol,
				Side:     types.SideBuy,
				At:       row.latestAt,
				Unit:     types.UnitQuoteCurrency,
				Raw:      row.fillBid.Float64(),
				Maturity: maturity,
				Validity: obsCtx.validity,
				Scale:    obsCtx.scale,
			})
		}

		if row.fillAsk != nil {
			out = append(out, &types.Measurement{
				Source:   types.SourceToxicity,
				Stream:   types.Toxicity,
				Metric:   types.MetricFillVolume,
				Subject:  types.SubjectLevel3Tape,
				Symbol:   symbol,
				Side:     types.SideSell,
				At:       row.latestAt,
				Unit:     types.UnitQuoteCurrency,
				Raw:      row.fillAsk.Float64(),
				Maturity: maturity,
				Validity: obsCtx.validity,
				Scale:    obsCtx.scale,
			})
		}

		for bookManager := range books {
			book := bookManager.GetBook(symbol)

			if book == nil || book.Name != symbol {
				continue
			}

			bid, ask := book.BestBid(), book.BestAsk()

			if bid == nil || ask == nil {
				continue
			}

			bidQuantity := bid.Quantity.Float64()
			askQuantity := ask.Quantity.Float64()

			nextTouch[symbol] = touchSnapshot{
				bidQuantity: bidQuantity,
				askQuantity: askQuantity,
				observedAt:  row.latestAt,
			}

			out = append(out,
				&types.Measurement{
					Source:   types.SourceToxicity,
					Stream:   types.Toxicity,
					Metric:   types.MetricBestPrice,
					Subject:  types.SubjectLevel3Touch,
					Symbol:   symbol,
					Side:     types.SideBuy,
					At:       row.latestAt,
					Unit:     types.UnitQuoteCurrency,
					Raw:      bid.Price.Float64(),
					Maturity: maturity,
					Validity: obsCtx.validity,
					Scale:    obsCtx.scale,
				},
				&types.Measurement{
					Source:   types.SourceToxicity,
					Stream:   types.Toxicity,
					Metric:   types.MetricBestPrice,
					Subject:  types.SubjectLevel3Touch,
					Symbol:   symbol,
					Side:     types.SideSell,
					At:       row.latestAt,
					Unit:     types.UnitQuoteCurrency,
					Raw:      ask.Price.Float64(),
					Maturity: maturity,
					Validity: obsCtx.validity,
					Scale:    obsCtx.scale,
				},
				&types.Measurement{
					Source:   types.SourceToxicity,
					Stream:   types.Toxicity,
					Metric:   types.MetricTouchQuantity,
					Subject:  types.SubjectLevel3Touch,
					Symbol:   symbol,
					Side:     types.SideBuy,
					At:       row.latestAt,
					Unit:     types.UnitBaseCurrency,
					Raw:      bidQuantity,
					Maturity: maturity,
					Validity: obsCtx.validity,
					Scale:    obsCtx.scale,
				},
				&types.Measurement{
					Source:   types.SourceToxicity,
					Stream:   types.Toxicity,
					Metric:   types.MetricTouchQuantity,
					Subject:  types.SubjectLevel3Touch,
					Symbol:   symbol,
					Side:     types.SideSell,
					At:       row.latestAt,
					Unit:     types.UnitBaseCurrency,
					Raw:      askQuantity,
					Maturity: maturity,
					Validity: obsCtx.validity,
					Scale:    obsCtx.scale,
				},
			)

			out = append(out, signal.touchHonesty(
				symbol,
				row,
				signal.priorTouch[symbol],
				bidQuantity,
				askQuantity,
				maturity,
			)...)
		}
	}

	signal.priorTouch = nextTouch
	signal.trades.cache = signal.trades.cache[:0]

	thesis.Signals.Store("trades", trades)
	thesis.Signals.Store("books", books)

	thesis.Measurements = append(thesis.Measurements, out...)

	return thesis
}

/*
touchHonesty compares current and prior touch quantities against executions so
cancellations are not mistaken for fills.
*/
func (signal *Signal) touchHonesty(
	symbol string,
	row *symbolEvidence,
	prior touchSnapshot,
	bidQuantity float64,
	askQuantity float64,
	maturity float64,
) []*types.Measurement {
	if prior.bidQuantity <= 0 && prior.askQuantity <= 0 {
		return nil
	}

	bidCancelled := cancelledQuantity(prior.bidQuantity, bidQuantity, row.bidExecuted)
	askCancelled := cancelledQuantity(prior.askQuantity, askQuantity, row.askExecuted)
	measurements := make([]*types.Measurement, 0, 4)
	obsCtx := newObservationContext(row.latestAt, row.tradeCount)

	if bidCancelled > 0 {
		measurements = append(measurements, &types.Measurement{
			Source:     types.SourceToxicity,
			Stream:     types.Toxicity,
			Metric:     types.MetricCancelledQuantity,
			Subject:    types.SubjectLevel3Touch,
			Symbol:     symbol,
			Side:       types.SideBuy,
			At:         row.latestAt,
			Unit:       types.UnitBaseCurrency,
			Raw:        bidCancelled,
			Normalized: types.NormalizeRatio(bidCancelled, prior.bidQuantity),
			Maturity:   maturity,
			Validity:   obsCtx.validity,
			Scale:      obsCtx.scale,
		})
	}

	if askCancelled > 0 {
		measurements = append(measurements, &types.Measurement{
			Source:     types.SourceToxicity,
			Stream:     types.Toxicity,
			Metric:     types.MetricCancelledQuantity,
			Subject:    types.SubjectLevel3Touch,
			Symbol:     symbol,
			Side:       types.SideSell,
			At:         row.latestAt,
			Unit:       types.UnitBaseCurrency,
			Raw:        askCancelled,
			Normalized: types.NormalizeRatio(askCancelled, prior.askQuantity),
			Maturity:   maturity,
			Validity:   obsCtx.validity,
			Scale:      obsCtx.scale,
		})
	}

	retreatingSide, retreatingQuantity, retreatBaseline := dominantRetreat(
		bidCancelled, askCancelled, prior.bidQuantity, prior.askQuantity,
	)

	if retreatingQuantity <= 0 {
		return measurements
	}

	measurements = append(measurements, &types.Measurement{
		Source:     types.SourceToxicity,
		Stream:     types.Toxicity,
		Metric:     types.MetricRetreatingQuantity,
		Subject:    types.SubjectLevel3Touch,
		Symbol:     symbol,
		Side:       retreatingSide,
		At:         row.latestAt,
		Unit:       types.UnitBaseCurrency,
		Raw:        retreatingQuantity,
		Normalized: types.NormalizeRatio(retreatingQuantity, retreatBaseline),
		Maturity:   maturity,
		Validity:   obsCtx.validity,
		Scale:      obsCtx.scale,
	})

	return measurements
}

func cancelledQuantity(
	priorQuantity float64,
	currentQuantity float64,
	executedQuantity float64,
) float64 {
	removal := priorQuantity - currentQuantity

	if removal <= 0 {
		return 0
	}

	return math.Max(0, removal-executedQuantity)
}

func dominantRetreat(
	bidCancelled float64,
	askCancelled float64,
	priorBid float64,
	priorAsk float64,
) (types.MeasurementSide, float64, float64) {
	bidPressure := retreatPressure(bidCancelled, priorBid)
	askPressure := retreatPressure(askCancelled, priorAsk)

	if bidPressure <= 0 && askPressure <= 0 {
		return types.SideNone, 0, 0
	}

	if askPressure > bidPressure {
		return types.SideSell, askCancelled, priorAsk
	}

	return types.SideBuy, bidCancelled, priorBid
}

func retreatPressure(cancelled float64, priorTouch float64) float64 {
	if cancelled <= 0 || priorTouch <= 0 {
		return 0
	}

	return cancelled / priorTouch
}

func tradeAt(trade spot.Trade) (time.Time, bool) {
	if trade.Time == nil || trade.Time.Sign() <= 0 {
		return time.Time{}, false
	}

	seconds := trade.Time.Float64()
	whole := int64(seconds)
	fraction := seconds - float64(whole)

	return time.Unix(whole, int64(fraction*1e9)).UTC(), true
}

/*
zeroed returns total, or a fresh zero accumulator when total has not been
seeded yet for this tick.
*/
func zeroed(total *decimal.Decimal) *decimal.Decimal {
	if total == nil {
		return decimal.NewFromFloat64(0)
	}

	return total
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
