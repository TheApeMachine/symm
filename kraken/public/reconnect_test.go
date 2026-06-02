package public

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestReconnectPolicyNextDelay(t *testing.T) {
	t.Cleanup(viper.Reset)

	convey.Convey("Given exponential reconnect backoff", t, func() {
		viper.Set("market.ws_reconnect_initial", 100*time.Millisecond)
		viper.Set("market.ws_reconnect_max", 400*time.Millisecond)
		viper.Set("market.ws_reconnect_multiplier", 2.0)

		policy := NewReconnectPolicyFromConfig()

		convey.Convey("It should grow delays until the cap", func() {
			convey.So(policy.NextDelay(), convey.ShouldEqual, 100*time.Millisecond)
			convey.So(policy.NextDelay(), convey.ShouldEqual, 200*time.Millisecond)
			convey.So(policy.NextDelay(), convey.ShouldEqual, 400*time.Millisecond)
			convey.So(policy.NextDelay(), convey.ShouldEqual, 400*time.Millisecond)
		})

		convey.Convey("It should reset after a successful connection", func() {
			policy.Reset()

			convey.So(policy.NextDelay(), convey.ShouldEqual, 100*time.Millisecond)
		})
	})
}

func TestCloneSubscribeFrame(t *testing.T) {
	convey.Convey("Given a subscribe frame", t, func() {
		cloned, ok := cloneSubscribeFrame(map[string]any{
			"method": "subscribe",
			"params": map[string]any{
				"channel": "instrument",
			},
		})

		convey.Convey("It should clone for replay", func() {
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(cloned["method"], convey.ShouldEqual, "subscribe")
		})
	})

	convey.Convey("Given a non-subscribe frame", t, func() {
		_, ok := cloneSubscribeFrame(map[string]any{"method": "ping"})

		convey.Convey("It should not be recorded", func() {
			convey.So(ok, convey.ShouldBeFalse)
		})
	})
}

func BenchmarkReconnectPolicyNextDelay(benchmark *testing.B) {
	policy := NewReconnectPolicyFromConfig()

	for benchmark.Loop() {
		policy.NextDelay()
	}
}
