package trader

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/tests/mockapi"
)

/*
BenchmarkMarketOnTrade measures decode-and-retain cost on the handler path.
*/
func BenchmarkMarketOnTrade(b *testing.B) {
	previousTimeline := viper.Get("signals.feed_timeline_capacity")
	previousTrack := viper.Get("signals.feed_track_capacity")
	b.Cleanup(func() { viper.Set("signals.feed_timeline_capacity", previousTimeline) })
	b.Cleanup(func() { viper.Set("signals.feed_track_capacity", previousTrack) })
	viper.Set("signals.feed_timeline_capacity", 128)
	viper.Set("signals.feed_track_capacity", 128)

	ctx := context.Background()
	mock := mockapi.NewMockAPI()
	api, _, err := mock.Wire(ctx)

	if err != nil {
		b.Fatal(err)
	}

	market, err := NewMarket(ctx, api, nil)

	if err != nil {
		b.Fatal(err)
	}

	defer market.Close()

	payload := []byte(`{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","side":"buy","price":100.0,"qty":0.1,"ord_type":"limit","trade_id":1,"timestamp":"2026-07-17T00:00:00.000000Z"}]}`)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		mock.Emit("trade", payload)
	}
}

/*
TestMarketTradeHandler verifies registration, Cut visibility, and Close.
*/
func TestMarketTradeHandler(t *testing.T) {
	previousTimeline := viper.Get("signals.feed_timeline_capacity")
	previousTrack := viper.Get("signals.feed_track_capacity")
	t.Cleanup(func() { viper.Set("signals.feed_timeline_capacity", previousTimeline) })
	t.Cleanup(func() { viper.Set("signals.feed_track_capacity", previousTrack) })
	viper.Set("signals.feed_timeline_capacity", 4)
	viper.Set("signals.feed_track_capacity", 4)

	Convey("Given a market wired through mock public ingress", t, func() {
		ctx := context.Background()
		mock := mockapi.NewMockAPI()
		api, _, err := mock.Wire(ctx)
		So(err, ShouldBeNil)

		market, err := NewMarket(ctx, api, nil)
		So(err, ShouldBeNil)

		payload := []byte(`{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","side":"buy","price":100.0,"qty":0.1,"ord_type":"limit","trade_id":1,"timestamp":"2026-07-17T00:00:00.000000Z"}]}`)
		cutAt := time.Date(2026, 7, 17, 0, 0, 1, 0, time.UTC)

		mock.Emit("trade", payload)

		Convey("When the handler retains the trade and Market closes", func() {
			frame, cutErr := market.Cut(cutAt)
			So(cutErr, ShouldBeNil)
			So(len(frame.Trades), ShouldEqual, 1)

			market.Close()
			mock.Emit("trade", payload)

			Convey("Then Close cancels the ingress context", func() {
				So(market.ctx.Err(), ShouldEqual, context.Canceled)
			})
		})
	})
}
