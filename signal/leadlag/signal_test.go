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

func TestLeadlagStep(t *testing.T) {
	Convey("Step records lead/lag pressure from trades", t, func() {
		signal := leadlag.NewSignal(context.Background())
		envelope := runtime.NewEnvelope(nil)
		leader := "LEAD/USD"
		follower := "FOL/USD"
		at := time.Now().UTC()

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

		reading := envelope.LeadLag[follower]
		So(reading, ShouldNotBeNil)
		So(reading.Eligible, ShouldBeTrue)
		So(reading.Category, ShouldEqual, 2)
		So(reading.Price, ShouldEqual, 37)
		So(reading.SyncScore, ShouldBeGreaterThan, 0.7)
		So(reading.DecoupledScore, ShouldEqual, 0)
		So(reading.InefficientScore, ShouldEqual, 0)
	})
}
