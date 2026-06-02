package public

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestLatencyProfileRoundTrip(t *testing.T) {
	Convey("Given a latency profile file", t, func() {
		path := filepath.Join(t.TempDir(), "network_latency.json")
		viper.Set("trading.paper.latency_profile", path)

		defer viper.Set("trading.paper.latency_profile", "")

		profile := LatencyProfile{
			RTTNS:     (120 * time.Millisecond).Nanoseconds(),
			OneWayNS:  (60 * time.Millisecond).Nanoseconds(),
			Samples:   8,
			UpdatedAt: time.Now().UTC(),
		}

		So(SaveLatencyProfile(profile), ShouldBeNil)

		loaded, ok := LoadLatencyProfile()

		Convey("It should reload RTT for replay scoring", func() {
			So(ok, ShouldBeTrue)
			So(loaded.RTT(), ShouldEqual, 120*time.Millisecond)
			So(loaded.OneWay(), ShouldEqual, 60*time.Millisecond)
		})
	})
}

func TestReplayExchangeLatencyUsesProfile(t *testing.T) {
	Convey("Given a persisted profile", t, func() {
		path := filepath.Join(t.TempDir(), "network_latency.json")
		viper.Set("trading.paper.latency_profile", path)

		defer viper.Set("trading.paper.latency_profile", "")

		So(SaveLatencyProfile(LatencyProfile{
			RTTNS: (88 * time.Millisecond).Nanoseconds(),
		}), ShouldBeNil)

		Convey("It should prefer stored RTT over live samples", func() {
			So(ReplayExchangeLatency(), ShouldEqual, 88*time.Millisecond)
		})
	})
}

func TestNetworkLatencyPersistProfile(t *testing.T) {
	Convey("Given a fresh latency measurement", t, func() {
		path := filepath.Join(t.TempDir(), "network_latency.json")
		viper.Set("trading.paper.latency_profile", path)

		defer viper.Set("trading.paper.latency_profile", "")

		latency := NewNetworkLatency()
		latency.RecordRTT(40 * time.Millisecond)

		_, err := os.Stat(path)

		Convey("It should write the profile file", func() {
			So(err, ShouldBeNil)
		})
	})
}
