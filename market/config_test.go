package market

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestScanSymbolCap(t *testing.T) {
	Convey("Given market.max_scan_symbols", t, func() {
		viper.Set("market.max_scan_symbols", 12)

		Convey("It should return the configured cap", func() {
			So(ScanSymbolCap(), ShouldEqual, 12)
		})

		viper.Set("market.max_scan_symbols", 0)

		Convey("It should apply the default when unset", func() {
			So(ScanSymbolCap(), ShouldEqual, defaultScanSymbolCap)
		})
	})
}

func TestRequiredBookDepthLevels(t *testing.T) {
	Convey("Given market.book_depth_levels", t, func() {
		viper.Set("market.book_depth_levels", 10)

		depth, err := RequiredBookDepthLevels()

		Convey("It should return the configured depth", func() {
			So(err, ShouldBeNil)
			So(depth, ShouldEqual, 10)
		})

		viper.Set("market.book_depth_levels", 0)

		_, err = RequiredBookDepthLevels()

		Convey("It should error when unset", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestRequiredDuration(t *testing.T) {
	Convey("Given trading.max_quote_age", t, func() {
		viper.Set("trading.max_quote_age", 5*time.Second)

		duration, err := RequiredDuration("trading.max_quote_age")

		Convey("It should return the configured duration", func() {
			So(err, ShouldBeNil)
			So(duration, ShouldEqual, 5*time.Second)
		})
	})
}
