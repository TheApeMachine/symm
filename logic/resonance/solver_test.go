package resonance

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestStep(t *testing.T) {
	Convey("Given a resonance solver with a synchronous observer", t, func() {
		solver := NewSolver(context.Background(), 0)
		defer solver.Close()
		observed := false
		solver.SetObserver(func(envelope *types.Envelope) {
			observed = envelope.Resonance != nil && envelope.Resonance.Manifold != nil
		})
		envelope := types.NewEnvelope(types.EnvelopeTicker)
		envelope.TickerData = kraken.TickerData{
			Symbol:    "TEST/USD",
			Bid:       decimal.NewFromFloat64(99),
			BidQty:    2,
			Ask:       decimal.NewFromFloat64(101),
			AskQty:    2,
			Last:      decimal.NewFromFloat64(100),
			Volume:    10,
			Vwap:      100,
			Low:       decimal.NewFromFloat64(98),
			High:      decimal.NewFromFloat64(102),
			Change:    decimal.NewFromFloat64(1),
			ChangePct: 1,
			Timestamp: time.Unix(1, 0),
		}

		result := solver.Step(envelope)

		Convey("the observer sees the owned model before it leaves the ring", func() {
			So(observed, ShouldBeTrue)
			So(result.Resonance, ShouldNotBeNil)
			So(result.Resonance.Manifold, ShouldBeNil)
		})
	})
}
