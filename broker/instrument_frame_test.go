package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestLevel3FrameSize(t *testing.T) {
	Convey("Given Kraken's standard level3 subscription-rate budget", t, func() {
		viper.Set("market.l3_rate_limit", 200)
		defer viper.Set("market.l3_rate_limit", nil)

		Convey("A frame carries only as many symbols as the budget pays for", func() {
			size, err := level3FrameSize(100)
			So(err, ShouldBeNil)
			So(size, ShouldEqual, 8)

			size, err = level3FrameSize(10)
			So(err, ShouldBeNil)
			So(size, ShouldEqual, 40)
		})

		Convey("An unsupported depth is an error", func() {
			_, err := level3FrameSize(25)
			So(err, ShouldNotBeNil)
		})
	})
}
