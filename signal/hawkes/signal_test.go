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

func TestNewSignal(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewSignal is called", func() {
			signal := hawkes.NewSignal(context.Background())

			Convey("It returns a usable signal", func() {
				So(signal, ShouldNotBeNil)
			})
		})
	})
}

func TestStep(t *testing.T) {
	Convey("Setup", t, func() {
		signal := hawkes.NewSignal(context.Background())
		envelope := runtime.NewEnvelope(nil)
		symbol := "HWK/USD"
		at := time.Unix(1_700_000_000, 0).UTC()

		Convey("When alternating buy and sell bursts arrive", func() {
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

			Convey("Hawkes fit is ready with mature excitation", func() {
				reading := envelope.Hawkes[symbol]
				So(reading, ShouldNotBeNil)
				So(reading.EventCount, ShouldEqual, 47)
				So(reading.BuyEventCount, ShouldEqual, 23)
				So(reading.SellEventCount, ShouldEqual, 24)
				So(reading.Readiness.HawkesFit, ShouldBeTrue)
				So(reading.Readiness.Intensity, ShouldBeTrue)
				So(reading.Fit.Valid(), ShouldBeTrue)
				So(reading.Maturity, ShouldEqual, 1)
			})
		})
	})
}
