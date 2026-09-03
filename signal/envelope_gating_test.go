package signal

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	nmdata "github.com/theapemachine/symm/nomagique/data"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"

	"github.com/theapemachine/symm/signal/cvd"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/signal/liquidity"
)

/*
The envelope-gating contract.

A signal observes exactly the envelope kind it consumes. This is not tidiness:
data.Lift discards the WHOLE frame when any one measurement carries an Err, so
a signal stepped on an envelope it cannot process poisons every other signal's
metrics on that same envelope.

That is what kept the advisors silent. Hawkes and CVD stepped on ticker
envelopes, received a zero-valued TradeData, correctly rejected it ("unsupported
trade side"), and the resulting Err erased pumpdump's and liquidity's metrics
from the ticker frame. No advisor could ever assemble a complete feature group,
so no perspective was issued, the War Room never had a participant, and the
planner returned "no advisor prediction has survived a round" forever.
*/

func TestEnvelopeGatingTest(t *testing.T) {
	Convey("Given a ticker envelope carrying no trade", t, func() {
		envelope := &types.Envelope{
			TypeID: types.EnvelopeTicker,
			TickerData: kraken.TickerData{
				Symbol:    "TEST/USD",
				Timestamp: time.Unix(1, 0).UTC(),
				Bid:       decimal.NewFromFloat64(100),
				Ask:       decimal.NewFromFloat64(101),
			},
		}

		Convey("A trade-clocked signal leaves it untouched", func() {
			hawkes.NewSignal(t.Context()).Step(envelope)
			cvd.NewSignal(t.Context(), nil).Step(envelope)

			So(envelope.Hawkes, ShouldBeNil)
			So(envelope.CVD, ShouldBeNil)
		})

		Convey("So a foreign signal cannot discard another's metrics", func() {
			hawkes.NewSignal(t.Context()).Step(envelope)
			cvd.NewSignal(t.Context(), nil).Step(envelope)

			// A real metric, as a ticker-clocked signal would have produced it.
			envelope.PumpDump = &nmdata.Measurement[float64]{
				Source: "pumpdump",
				Metrics: map[string]nmdata.Metric[float64]{
					"midpoint_log_return": {Raw: 0.0125},
				},
			}

			frame := nmdata.Lift(envelope.SignalMeasurements())

			// The whole point: one signal stepped out of turn must not be able
			// to erase the metrics every other signal produced here.
			So(frame.Err, ShouldBeNil)
			So(frame.Has(nmtypes.MustIntern("pumpdump/midpoint_log_return")), ShouldBeTrue)
		})
	})

	Convey("Given a trade envelope carrying no ticker", t, func() {
		envelope := &types.Envelope{
			TypeID: types.EnvelopeTrade,
			TradeData: kraken.TradeData{
				Symbol:    "TEST/USD",
				Side:      "buy",
				Timestamp: time.Unix(1, 0).UTC(),
			},
		}

		Convey("A ticker-clocked signal leaves it untouched", func() {
			liquidity.NewSignal(t.Context()).Step(envelope)

			So(envelope.Liquidity, ShouldBeNil)
		})

		Convey("And the trade-clocked signals still observe it", func() {
			hawkes.NewSignal(t.Context()).Step(envelope)

			So(envelope.Hawkes, ShouldNotBeNil)
		})
	})
}
