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
		level3:     NewLevel3(ctx, api),
		priorTouch: map[string]touchSnapshot{},
	}
}

type touchSnapshot struct {
	bidQuantity float64
	askQuantity float64
	observedAt  time.Time
}

type symbolEvidence struct {
	latestAt    time.Time
	tradeCount  int
	volume      *decimal.Decimal
	fillBid     *decimal.Decimal
	fillAsk     *decimal.Decimal
	bidExecuted float64
	askExecuted float64
}

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

		out = append(out,
			types.ObservationMeasurement(
				types.SourceToxicity, types.Toxicity, types.MetricTradeVolume,
				types.SubjectLevel3Tape, symbol, row.latestAt,
				types.UnitBaseCurrency, row.volume.Float64(), maturity,
			),
		)

		if row.fillBid != nil {
			out = append(out,
				types.ObservationSideMeasurement(
					types.SourceToxicity, types.Toxicity, types.MetricFillVolume,
					types.SubjectLevel3Tape, symbol, types.SideBuy, row.latestAt,
					types.UnitQuoteCurrency, row.fillBid.Float64(), maturity,
				),
			)
		}

		if row.fillAsk != nil {
			out = append(out,
				types.ObservationSideMeasurement(
					types.SourceToxicity, types.Toxicity, types.MetricFillVolume,
					types.SubjectLevel3Tape, symbol, types.SideSell, row.latestAt,
					types.UnitQuoteCurrency, row.fillAsk.Float64(), maturity,
				),
			)
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
				types.ObservationSideMeasurement(
					types.SourceToxicity, types.Toxicity, types.MetricBestPrice,
					types.SubjectLevel3Touch, symbol, types.SideBuy, row.latestAt,
					types.UnitQuoteCurrency, bid.Price.Float64(), maturity,
				),
				types.ObservationSideMeasurement(
					types.SourceToxicity, types.Toxicity, types.MetricBestPrice,
					types.SubjectLevel3Touch, symbol, types.SideSell, row.latestAt,
					types.UnitQuoteCurrency, ask.Price.Float64(), maturity,
				),
				types.ObservationSideMeasurement(
					types.SourceToxicity, types.Toxicity, types.MetricTouchQuantity,
					types.SubjectLevel3Touch, symbol, types.SideBuy, row.latestAt,
					types.UnitBaseCurrency, bidQuantity, maturity,
				),
				types.ObservationSideMeasurement(
					types.SourceToxicity, types.Toxicity, types.MetricTouchQuantity,
					types.SubjectLevel3Touch, symbol, types.SideSell, row.latestAt,
					types.UnitBaseCurrency, askQuantity, maturity,
				),
			)

			out = append(out, signal.touchHonesty(
				symbol, row, signal.priorTouch[symbol],
				bidQuantity, askQuantity, maturity,
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

	if bidCancelled > 0 {
		measurements = append(measurements,
			types.ObservationSideNormalizedMeasurement(
				types.SourceToxicity, types.Toxicity, types.MetricCancelledQuantity,
				types.SubjectLevel3Touch, symbol, types.SideBuy, row.latestAt,
				types.UnitBaseCurrency, bidCancelled, maturity,
				types.NormalizeRatio(bidCancelled, prior.bidQuantity),
			),
		)
	}

	if askCancelled > 0 {
		measurements = append(measurements,
			types.ObservationSideNormalizedMeasurement(
				types.SourceToxicity, types.Toxicity, types.MetricCancelledQuantity,
				types.SubjectLevel3Touch, symbol, types.SideSell, row.latestAt,
				types.UnitBaseCurrency, askCancelled, maturity,
				types.NormalizeRatio(askCancelled, prior.askQuantity),
			),
		)
	}

	retreatingSide, retreatingQuantity, retreatBaseline := dominantRetreat(
		bidCancelled, askCancelled, prior.bidQuantity, prior.askQuantity,
	)

	if retreatingQuantity <= 0 {
		return measurements
	}

	measurements = append(measurements,
		types.ObservationSideNormalizedMeasurement(
			types.SourceToxicity, types.Toxicity, types.MetricRetreatingQuantity,
			types.SubjectLevel3Touch, symbol, retreatingSide, row.latestAt,
			types.UnitBaseCurrency, retreatingQuantity, maturity,
			types.NormalizeRatio(retreatingQuantity, retreatBaseline),
		),
	)

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

func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
