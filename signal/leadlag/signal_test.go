package leadlag_test

import (
	"context"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/signal/leadlag"
)

func TestNewSignal(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewSignal is called", func() {
			signal := leadlag.NewSignal(context.Background())

			Convey("It returns a usable signal", func() {
				So(signal, ShouldNotBeNil)
			})
		})
	})
}

func TestStep(t *testing.T) {
	Convey("Setup", t, func() {
		signal := leadlag.NewSignal(context.Background())
		envelope := runtime.NewEnvelope(nil)
		leader := "LEAD/USD"
		follower := "FOL/USD"
		at := time.Unix(1_700_000_000, 0).UTC()

		Convey("When leader prints move before the follower", func() {
			for i := range 32 {
				leaderPrice := decimal.NewFromInt(10 + int64(i))
				followerPrice := decimal.NewFromInt(10)
				if i > 4 {
					followerPrice = decimal.NewFromInt(10 + int64(i-4))
				}

				envelope.Trade = &market.Trade{Data: []*market.TradeData{
					{
						Symbol:    leader,
						Side:      market.SideBuy,
						Price:     leaderPrice,
						Qty:       decimal.NewFromInt(2),
						Timestamp: at.Add(time.Duration(i) * time.Second),
					},
					{
						Symbol:    follower,
						Side:      market.SideBuy,
						Price:     followerPrice,
						Qty:       decimal.NewFromInt(1),
						Timestamp: at.Add(time.Duration(i) * time.Second),
					},
				}}
				envelope = signal.Step(envelope)
			}

			Convey("Follower is eligible sync with decoupled score zero", func() {
				reading := envelope.LeadLag[follower]
				So(reading, ShouldNotBeNil)
				So(reading.Eligible, ShouldBeTrue)
				So(reading.Category, ShouldEqual, 2)
				So(reading.Price, ShouldEqual, 37)
				So(reading.SyncScore, ShouldEqual, 0.7810685870840293)
				So(reading.DecoupledScore, ShouldEqual, 0)
				So(reading.InefficientScore, ShouldEqual, 0)
			})
		})
	})
}
