package trader

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestCryptoPulseForecastError(t *testing.T) {
	convey.Convey("Given sequential engine pulses", t, func() {
		crypto := newTestCrypto()

		firstError, firstMultiple := crypto.pulseForecastError(0.5, 0.01)
		secondError, secondMultiple := crypto.pulseForecastError(0.6, 0.01)

		convey.Convey("It should leave the first pulse without error and measure the second miss", func() {
			convey.So(firstError, convey.ShouldEqual, 0)
			convey.So(firstMultiple, convey.ShouldEqual, 0)
			convey.So(secondError, convey.ShouldAlmostEqual, 0.1, 0.0001)
			convey.So(secondMultiple, convey.ShouldAlmostEqual, 10, 0.0001)
		})
	})
}
