package engine

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/market"
)

func TestMarketRelayRead(t *testing.T) {
	Convey("Given a market relay subscribed to broadcast groups", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 1, 2, qpool.NewConfig())
		defer pool.Close()

		tickGroup := pool.CreateBroadcastGroup("tick", 10*time.Millisecond)
		tradeGroup := pool.CreateBroadcastGroup("trade", 10*time.Millisecond)
		bookGroup := pool.CreateBroadcastGroup("book", 10*time.Millisecond)

		relay, err := NewMarketRelay(ctx, tickGroup, tradeGroup, bookGroup)
		So(err, ShouldBeNil)

		tickGroup.Send(&qpool.QValue[any]{
			SenderID: "test",
			Value: TickUpdate{
				Symbol:     "PUMP/EUR",
				Last:       1.25,
				VolumeBase: 500000,
				ChangePct:  2.5,
				Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
			},
		})

		Convey("When Tick drains the update", func() {
			So(relay.Drain(1), ShouldEqual, 1)

			snapshot := relay.Read("PUMP/EUR")

			Convey("It should expose the cached ticker snapshot", func() {
				So(snapshot.LastOK, ShouldBeTrue)
				So(snapshot.Last, ShouldEqual, 1.25)
				So(snapshot.VolumeOK, ShouldBeTrue)
				So(snapshot.ChangeOK, ShouldBeTrue)
				So(snapshot.ChangePct, ShouldEqual, 2.5)
			})
		})
	})
}

func TestMarketRelayRecentTicks(t *testing.T) {
	Convey("Given trade updates on the broadcast group", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 1, 2, qpool.NewConfig())
		defer pool.Close()

		tradeGroup := pool.CreateBroadcastGroup("trade", 10*time.Millisecond)
		tickGroup := pool.CreateBroadcastGroup("tick", 10*time.Millisecond)
		bookGroup := pool.CreateBroadcastGroup("book", 10*time.Millisecond)

		relay, err := NewMarketRelay(ctx, tickGroup, tradeGroup, bookGroup)
		So(err, ShouldBeNil)

		tradeAt := time.Unix(1000, 0).UTC()
		tradeGroup.Send(&qpool.QValue[any]{
			SenderID: "test",
			Value: TradeUpdate{
				Symbol:      "PUMP/EUR",
				BatchVolume: 4,
				BuyPressure: 0.5,
				UpdatedAt:   tradeAt,
				Ticks: []market.TradeTick{
					{Symbol: "PUMP/EUR", Side: "buy", Volume: 3, Timestamp: tradeAt},
					{Symbol: "PUMP/EUR", Side: "sell", Volume: 1, Timestamp: tradeAt},
				},
			},
		})

		Convey("When Tick drains the trade batch", func() {
			So(relay.Drain(1), ShouldEqual, 1)

			ticks, ok := relay.RecentTicks("PUMP/EUR", time.Time{})

			Convey("It should retain per-trade events for Hawkes fitting", func() {
				So(ok, ShouldBeTrue)
				So(len(ticks), ShouldEqual, 2)
			})
		})
	})
}

func TestMarketRelayReadFresh(t *testing.T) {
	Convey("Given a stale cached snapshot", t, func() {
		ctx := context.Background()
		pool := qpool.NewQ(ctx, 1, 2, qpool.NewConfig())
		defer pool.Close()

		tickGroup := pool.CreateBroadcastGroup("tick", 10*time.Millisecond)
		tradeGroup := pool.CreateBroadcastGroup("trade", 10*time.Millisecond)
		bookGroup := pool.CreateBroadcastGroup("book", 10*time.Millisecond)

		relay, err := NewMarketRelay(ctx, tickGroup, tradeGroup, bookGroup)
		So(err, ShouldBeNil)

		staleAt := time.Unix(1_000, 0).UTC()
		tickGroup.Send(&qpool.QValue[any]{
			SenderID: "test",
			Value: TickUpdate{
				Symbol:     "PUMP/EUR",
				Last:       1.25,
				VolumeBase: 500000,
				ChangePct:  2.5,
				Timestamp:  staleAt.Format(time.RFC3339Nano),
			},
		})

		So(relay.Tick(), ShouldBeNil)

		now := staleAt.Add(100 * time.Millisecond)
		snapshot := relay.ReadFresh("PUMP/EUR", now, 10*time.Millisecond)

		Convey("It should clear stale ticker fields", func() {
			So(snapshot.LastOK, ShouldBeFalse)
		})
	})
}

func BenchmarkMarketRelayRead(b *testing.B) {
	ctx := context.Background()
	pool := qpool.NewQ(ctx, 1, 2, qpool.NewConfig())
	defer pool.Close()

	tickGroup := pool.CreateBroadcastGroup("tick", 10*time.Millisecond)
	tradeGroup := pool.CreateBroadcastGroup("trade", 10*time.Millisecond)
	bookGroup := pool.CreateBroadcastGroup("book", 10*time.Millisecond)

	relay, err := NewMarketRelay(ctx, tickGroup, tradeGroup, bookGroup)

	if err != nil {
		b.Fatal(err)
	}

	tickGroup.Send(&qpool.QValue[any]{
		SenderID: "test",
		Value: TickUpdate{
			Symbol:     "PUMP/EUR",
			Last:       1.25,
			VolumeBase: 500000,
			ChangePct:  2.5,
		},
	})

	for relay.Drain(1) > 0 {
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = relay.Read("PUMP/EUR")
	}
}
