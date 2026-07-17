package trader

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

/*
TestIngressBufferSizeFloor keeps the public decode offload from collapsing to a
single-slot queue when config is undersized.
*/
func TestIngressBufferSizeFloor(t *testing.T) {
	previous := viper.Get("system.websocket.channel.buffer")
	t.Cleanup(func() { viper.Set("system.websocket.channel.buffer", previous) })

	Convey("Given a tiny configured buffer", t, func() {
		viper.Set("system.websocket.channel.buffer", 1)
		So(ingressBufferSize(), ShouldEqual, 64)
	})

	Convey("Given a large configured buffer", t, func() {
		viper.Set("system.websocket.channel.buffer", 4096)
		So(ingressBufferSize(), ShouldEqual, 4096)
	})
}

/*
BenchmarkMarketOnTrade measures decode-and-retain cost on the worker path so
ingress offload regressions show up in allocation pressure.
*/
func BenchmarkMarketOnTrade(b *testing.B) {
	previousTimeline := viper.Get("signals.feed_timeline_capacity")
	previousTrack := viper.Get("signals.feed_track_capacity")
	b.Cleanup(func() { viper.Set("signals.feed_timeline_capacity", previousTimeline) })
	b.Cleanup(func() { viper.Set("signals.feed_track_capacity", previousTrack) })
	viper.Set("signals.feed_timeline_capacity", 128)
	viper.Set("signals.feed_track_capacity", 128)

	market, err := NewMarket(nil, nil)
	if err != nil {
		b.Fatal(err)
	}

	payload := []byte(`{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","side":"buy","price":100.0,"qty":0.1,"ord_type":"limit","trade_id":1,"timestamp":"2026-07-17T00:00:00.000000Z"}]}`)
	_ = time.Now()

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		market.OnTrade(payload)
	}
}
