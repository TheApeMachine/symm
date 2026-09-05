package advisor

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	nmdata "github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/types"

	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/pumpdump"
)

/*
The advisor pipeline, end to end over the real signals.

This is the path that was silently dead. Every advisor gates on
vector.GroupsComplete, which needs every declared feature metric present in one
accumulated frame. Metrics arrive across envelope kinds — pumpdump on both
ticker and trade, hawkes and cvd on trade — and the solver accumulates them per
symbol between clock ticks.

What broke it was upstream: hawkes and cvd stepped on ticker envelopes too,
were handed a zero-valued TradeData, correctly rejected it, and the resulting
Err made data.Lift discard the entire ticker frame — pumpdump's metrics
included. The accumulator therefore never grew past whatever a single trade
envelope carried, no feature group ever completed, and no perspective was ever
issued.

The assertion here is deliberately about frames rather than about a specific
advisor firing: an advisor needs many volume bars of real history to classify,
which is not what this test is pinning down. What it pins down is that metrics
from different envelope kinds now survive into one frame together.
*/

const pipelineSymbol = "TEST/USD"

func tickerEnvelope(at time.Time, bid, ask float64) *types.Envelope {
	return &types.Envelope{
		TypeID: types.EnvelopeTicker,
		TickerData: kraken.TickerData{
			Symbol:    pipelineSymbol,
			Timestamp: at,
			Bid:       decimal.NewFromFloat64(bid),
			Ask:       decimal.NewFromFloat64(ask),
			Last:      decimal.NewFromFloat64(bid),
		},
	}
}

func tradeEnvelope(at time.Time, price, quantity float64, side string) *types.Envelope {
	return &types.Envelope{
		TypeID: types.EnvelopeTrade,
		TradeData: kraken.TradeData{
			Symbol:    pipelineSymbol,
			Timestamp: at,
			Side:      side,
			Price:     *decimal.NewFromFloat64(price),
			Qty:       quantity,
			TradeID:   int64(at.Unix()),
		},
	}
}

func TestAdvisorPipelineTest(t *testing.T) {
	Convey("Given the real trade- and ticker-clocked signals", t, func() {
		hawkesSignal := hawkes.NewSignal(t.Context())
		cvdSignal := cvd.NewSignal(t.Context(), nil)
		pumpSignal := pumpdump.NewSignal(t.Context(), nil)

		start := time.Unix(1_700_000_000, 0).UTC()

		step := func(envelope *types.Envelope) *types.Envelope {
			pumpSignal.Step(envelope)
			hawkesSignal.Step(envelope)
			cvdSignal.Step(envelope)

			return envelope
		}

		lift := func(envelope *types.Envelope) (map[string]float64, error) {
			measurements := envelope.SignalMeasurements()

			return nmdata.Lift(measurements[:])
		}

		Convey("A ticker envelope lifts cleanly, carrying its own metrics", func() {
			envelope := step(tickerEnvelope(start, 100, 100.5))
			observation, err := lift(envelope)

			// Before the fix this was non-nil: hawkes and cvd rejected the
			// absent trade and erased pumpdump's ticker metrics with it.
			So(err, ShouldBeNil)
			So(len(observation), ShouldBeGreaterThan, 0)
			So(envelope.PumpDump, ShouldNotBeNil)
			So(envelope.Hawkes, ShouldBeNil)
			So(envelope.CVD, ShouldBeNil)
		})

		Convey("A trade envelope lifts cleanly and carries the trade signals", func() {
			step(tickerEnvelope(start, 100, 100.5))

			envelope := step(tradeEnvelope(
				start.Add(time.Second), 100.2, 5, "buy",
			))
			observation, err := lift(envelope)

			So(err, ShouldBeNil)
			So(len(observation), ShouldBeGreaterThan, 0)
			So(envelope.Hawkes, ShouldNotBeNil)
		})

		Convey("Metrics from both envelope kinds accumulate into one observation", func() {
			accumulated, err := lift(step(tickerEnvelope(start, 100, 100.5)))
			So(err, ShouldBeNil)

			// A signal that legitimately rejected its observation carries an
			// Err; Lift skips it and keeps every other signal's metrics, which
			// is the whole point of the fix this test guards.
			merge := func(envelope *types.Envelope) {
				observation, _ := lift(envelope)

				for key, value := range observation {
					accumulated[key] = value
				}
			}

			for index := 1; index <= 12; index++ {
				at := start.Add(time.Duration(index) * time.Second)

				merge(step(tradeEnvelope(at, 100+float64(index)*0.1, 5, "buy")))
				merge(step(tickerEnvelope(at, 100+float64(index)*0.1, 100.6+float64(index)*0.1)))
			}

			// The accumulator must now hold metrics sourced from BOTH clocks
			// at once. That co-presence is precisely what a complete feature
			// group needs, and precisely what the lift abort used to make
			// impossible.
			So(len(accumulated), ShouldBeGreaterThan, 20)
		})
	})
}

/*
The two venues Kraken names the same market on.

Basis declares nothing but derivatives metrics plus the spot volume-bar clock.
Derivatives are produced only on futures envelopes, whose identity is the
product ("PF_SOLUSD"); the clock is produced only on spot trades, whose
identity is the pair ("SOL/USD"). A solver accumulates evidence per symbol, so
the two halves of Basis's contract were filed under two different keys and
never met — which is why it was mute on every one of 627 instruments rather
than merely on the ones without a futures market.
*/
func futuresTickerEnvelope(at time.Time, product string) *types.Envelope {
	return &types.Envelope{
		TypeID: types.EnvelopeFuturesTicker,
		FuturesTickerData: kraken.FuturesTickerData{
			ProductID: product,
			Symbol:    product,
			Timestamp: at,
		},
	}
}

func TestFuturesEvidenceReachesTheSpotSymbol(t *testing.T) {
	Convey("Given a futures envelope and the spot pair it derives from", t, func() {
		at := time.Unix(1_700_000_000, 0).UTC()

		Convey("the solver files the derivative under the spot symbol", func() {
			So(
				envelopeSymbol(futuresTickerEnvelope(at, "PF_SOLUSD")),
				ShouldEqual,
				"SOL/USD",
			)
		})

		Convey("a spot trade on that pair files under the same symbol", func() {
			envelope := &types.Envelope{
				TypeID: types.EnvelopeTrade,
				TradeData: kraken.TradeData{
					Symbol:    "SOL/USD",
					Timestamp: at,
					Side:      "buy",
					Price:     *decimal.NewFromFloat64(100),
					Qty:       5,
				},
			}

			So(envelopeSymbol(envelope), ShouldEqual, "SOL/USD")
		})

		Convey("so both venues accumulate into one observation", func() {
			solver := NewSolver(t.Context(), BasisName, NewBasis().Features)

			solver.observe("SOL/USD", map[string]float64{
				"derivatives/basis": 0.0012,
			})
			solver.observe(
				envelopeSymbol(futuresTickerEnvelope(at, "PF_SOLUSD")),
				map[string]float64{"derivatives/open_interest": 4200},
			)

			observation := solver.observe("SOL/USD", map[string]float64{
				"pumpdump/completed_volume_bar_ordinal": 7,
			})

			So(observation, ShouldContainKey, "derivatives/basis")
			So(observation, ShouldContainKey, "derivatives/open_interest")
			So(observation, ShouldContainKey, "pumpdump/completed_volume_bar_ordinal")
		})

		Convey("a product this venue does not name stays under its own identity", func() {
			So(
				envelopeSymbol(futuresTickerEnvelope(at, "WEIRD-THING")),
				ShouldEqual,
				"WEIRD-THING",
			)
		})
	})
}
