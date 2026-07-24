package stack_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
)

/*
TestBooterConfigureTestIsolation proves configureTest overrides ambient viper
without leaving the production graph on a depth-one channel buffer.
*/
func TestBooterConfigureTestIsolation(t *testing.T) {
	Convey("Given ambient configuration that conflicts with the test market", t, func() {
		previousBuffer := viper.Get("system.websocket.channel.buffer")
		viper.Set("system.websocket.channel.buffer", 1)
		market := tests.NewMarket(t.Context(), 1)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		Reset(func() {
			if wired != nil {
				So(wired.Close(), ShouldBeNil)
			}

			market.Close()
			viper.Set("system.websocket.channel.buffer", previousBuffer)
		})

		Convey("The graph should use only its deterministic test configuration", func() {
			So(viper.GetInt("system.websocket.channel.buffer"), ShouldEqual, 4096)
			So(cap(wired.Channel), ShouldEqual, 4096)
			So(wired.Close(), ShouldBeNil)
			wired = nil
			So(viper.GetInt("system.websocket.channel.buffer"), ShouldEqual, 1)
		})
	})
}
