package hawkes_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/signal/hawkes"
)

func TestHawkesStep(t *testing.T) {
	Convey("Step fits bivariate Hawkes after alternating buy and sell bursts", t, func() {
		signal := hawkes.NewSignal(context.Background())
		envelope := runtime.NewEnvelope(nil)
		symbol := "HWK/USD"
		at := time.Now().UTC()

		for i := range 48 {
			side := market.SideBuy
			if i%2 == 1 {
				side = market.SideSell
			}

			envelope.Trade = &market.Trade{Data: []*market.TradeData{{
				Symbol:    symbol,
				Side:      side,
				Price:     decimal.NewFromInt(5),
				Qty:       decimal.NewFromInt(2),
				Timestamp: at.Add(time.Duration(i) * time.Millisecond),
			}}}
			envelope = signal.Step(envelope)
		}

		reading := envelope.Hawkes[symbol]
		So(reading, ShouldNotBeNil)
		So(reading.EventCount, ShouldBeGreaterThan, 40)
		So(reading.BuyEventCount, ShouldBeGreaterThan, 15)
		So(reading.SellEventCount, ShouldBeGreaterThan, 15)
		So(reading.Readiness.HawkesFit, ShouldBeTrue)
		So(reading.Readiness.Intensity, ShouldBeTrue)
		So(reading.Fit.Valid(), ShouldBeTrue)
		So(reading.Maturity, ShouldEqual, 1)
	})
}
