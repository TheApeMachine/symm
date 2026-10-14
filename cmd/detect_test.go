package cmd

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/system"
)

func TestDetectCmd(t *testing.T) {
	Convey("Given detect command configuration", t, func() {
		system.NewConfig()

		Convey("The command defines its required flags", func() {
			So(detectCmd.Use, ShouldEqual, "detect")
			So(detectCmd.Flag("epoch"), ShouldNotBeNil)
			So(detectCmd.Flag("symbols"), ShouldNotBeNil)
			So(detectCmd.Flag("taker-fee-percent"), ShouldNotBeNil)
		})

		Convey("offlinePrice fails when fee is invalid or balance is non-positive", func() {
			ctx := t.Context()
			price, err := offlinePrice(ctx, "-1.0", []string{"BTC/USD"})
			So(err, ShouldNotBeNil)
			So(price, ShouldBeNil)

			price, err = offlinePrice(ctx, "0.26", []string{"BTC/USD"})
			So(err, ShouldBeNil)
			So(price, ShouldNotBeNil)
		})
	})
}
