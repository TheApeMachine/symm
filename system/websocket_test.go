package system

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewWebSocket(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewWebSocket is called", func() {
			ws := NewWebSocket()

			Convey("Kraken endpoints and refresh knobs match viper defaults", func() {
				So(ws.Endpoints.Public, ShouldEqual, "wss://ws.kraken.com/v2")
				So(ws.Endpoints.Private, ShouldEqual, "wss://ws-auth.kraken.com/v2")
				So(ws.Endpoints.Level3, ShouldEqual, "wss://ws-l3.kraken.com/v2")
				So(ws.PingInterval, ShouldEqual, 20*time.Second)
				So(ws.PrivateRefresh, ShouldEqual, 5*time.Second)
				So(ws.TickerRefresh, ShouldEqual, 30*time.Second)
				So(ws.TickerBatchSize, ShouldEqual, 128)
			})
		})
	})
}

func BenchmarkNewWebSocket(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = NewWebSocket()
	}
}
