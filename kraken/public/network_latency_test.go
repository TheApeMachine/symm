package public

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestNetworkLatencyOneWay(t *testing.T) {
	Convey("Given measured RTT samples", t, func() {
		latency := NewNetworkLatency()
		latency.RecordRTT(100 * time.Millisecond)

		Convey("It should expose p95 one-way latency", func() {
			So(latency.OneWay(), ShouldEqual, 50*time.Millisecond)
		})
	})

	Convey("Given a latency burst in the tail", t, func() {
		latency := NewNetworkLatency()

		for range 20 {
			latency.RecordRTT(100 * time.Millisecond)
		}

		for range 5 {
			latency.RecordRTT(200 * time.Millisecond)
		}

		Convey("It should use p95 RTT instead of the mean", func() {
			So(latency.OneWay(), ShouldEqual, 100*time.Millisecond)
			So(latency.OneWay(), ShouldBeGreaterThan, latency.MeanRTT()/2)
		})
	})

	Convey("Given no samples and configured fallback", t, func() {
		viper.Set("trading.paper.default_one_way_latency", 25*time.Millisecond)

		defer viper.Set("trading.paper.default_one_way_latency", time.Duration(0))

		latency := NewNetworkLatency()

		Convey("It should use the configured fallback", func() {
			So(latency.OneWay(), ShouldEqual, 25*time.Millisecond)
		})
	})
}
