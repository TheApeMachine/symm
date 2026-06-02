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

		Convey("It should expose half RTT as one-way latency", func() {
			So(latency.OneWay(), ShouldEqual, 50*time.Millisecond)
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
