package toxicity

import (
	"context"
	"math"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
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
	level3     *Level3
	priorTouch map[string]touchSnapshot
	ui         chan []byte
}

func NewSignal(ctx context.Context, api *websocket.API, ui chan []byte) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:        ctx,
		cancel:     cancel,
		level3:     NewLevel3(api),
		priorTouch: map[string]touchSnapshot{},
		ui:         ui,
	}
}

/*
Publish sends one small datura frame to the UI the moment this signal has
measured its evidence, mirroring broker.Balance.Publish.
*/
func (signal *Signal) Publish(measurements []*types.Measurement) {
	select {
	case signal.ui <- datura.Map[any]{
		"measurements": types.WireMeasurements(measurements),
	}.Marshal():
	default:
	}
}

func (signal *Signal) Measure(thesis *types.Thesis) []*types.Measurement {
	measurements, err := signal.Calculate(thesis.Market())

	if err != nil {
		errnie.Error(err)
		return nil
	}

	return measurements
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
Calculate converts the receiver's current market input into typed measurements
so downstream logic consumes explicit evidence.
*/
func (signal *Signal) Calculate(
	frame *types.MarketFrame,
) ([]*types.Measurement, error) {
	trades := frame.Trades
	incrementBySymbol := map[string]*decimal.Decimal{}

	for _, row := range frame.Books {
		if row.Symbol == "" || row.PriceIncrement.Sign() <= 0 {
			continue
		}

		increment := row.PriceIncrement
		incrementBySymbol[row.Symbol] = &increment
	}

	evidence := map[string]*symbolEvidence{}

	for _, trade := range trades {
		if trade.Price.Sign() <= 0 || trade.Qty <= 0 || trade.Timestamp.IsZero() {
			continue
		}

		at := trade.Timestamp.UTC()

		row := evidence[trade.Symbol]

		if row == nil {
			row = &symbolEvidence{}
			evidence[trade.Symbol] = row
		}

		row.tradeCount++
		volume := decimal.NewFromFloat64(trade.Qty)
		row.volume = zeroed(row.volume).Add(volume)

		if at.After(row.latestAt) {
			row.latestAt = at
		}

		increment := incrementBySymbol[trade.Symbol]

		signal.level3.PeekBook(trade.Symbol, func(symbolBook *book.Book) {
			if symbolBook.Name != trade.Symbol {
				return
			}

			bid, ask := symbolBook.BestBid(), symbolBook.BestAsk()

			if bid == nil || ask == nil {
				return
			}

			if touchMatch(trade.Price, bid.Price, increment) {
				row.fillBid = zeroed(row.fillBid).Add(bid.Price.Mul(volume))
				row.bidExecuted += trade.Qty
			}

			if touchMatch(trade.Price, ask.Price, increment) {
				row.fillAsk = zeroed(row.fillAsk).Add(ask.Price.Mul(volume))
				row.askExecuted += trade.Qty
			}
		})
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

		signal.level3.PeekBook(symbol, func(symbolBook *book.Book) {
			if symbolBook.Name != symbol {
				return
			}

			bid, ask := symbolBook.BestBid(), symbolBook.BestAsk()

			if bid == nil || ask == nil {
				return
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
					Source:  types.SourceToxicity,
					Stream:  types.Toxicity,
					Metric:  types.MetricTouchQuantity,
					Subject: types.SubjectLevel3Touch,
					Symbol:  symbol,
					Side:    types.SideBuy,
					At:      row.latestAt,
					Unit:    types.UnitBaseCurrency,
					Raw:     bidQuantity,
					Normalized: types.NormalizeRatio(
						bidQuantity, bidQuantity+askQuantity,
					),
					Maturity: maturity,
					Validity: obsCtx.validity,
					Scale:    obsCtx.scale,
				},
				&types.Measurement{
					Source:  types.SourceToxicity,
					Stream:  types.Toxicity,
					Metric:  types.MetricTouchQuantity,
					Subject: types.SubjectLevel3Touch,
					Symbol:  symbol,
					Side:    types.SideSell,
					At:      row.latestAt,
					Unit:    types.UnitBaseCurrency,
					Raw:     askQuantity,
					Normalized: types.NormalizeRatio(
						askQuantity, bidQuantity+askQuantity,
					),
					Maturity: maturity,
					Validity: obsCtx.validity,
					Scale:    obsCtx.scale,
				},
			)

			measurements := signal.touchHonesty(
				symbol,
				row,
				signal.priorTouch[symbol],
				bidQuantity,
				askQuantity,
				maturity,
			)

			out = append(out, measurements...)
		})
	}

	signal.Publish(out)
	signal.priorTouch = nextTouch

	return out, nil
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

func touchMatch(tradePrice decimal.Decimal, touchPrice *decimal.Decimal, increment *decimal.Decimal) bool {
	if touchPrice == nil {
		return false
	}

	if increment == nil || increment.Sign() <= 0 {
		return tradePrice.Cmp(touchPrice) == 0
	}

	tradeTick, err := kraken.PriceTick(tradePrice, *increment)

	if err != nil {
		return tradePrice.Cmp(touchPrice) == 0
	}

	touchTick, err := kraken.PriceTick(*touchPrice, *increment)

	if err != nil {
		return tradePrice.Cmp(touchPrice) == 0
	}

	delta := tradeTick - touchTick

	if delta < 0 {
		delta = -delta
	}

	return delta <= 1
}

/*
Close releases the receiver's owned resources so shutdown does not leave
active market-data producers.
*/
func (signal *Signal) Close() error {
	signal.cancel()
	return nil
}
