package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/decimal"
)

func TestScaledBookDecimalKeepsPairScale(t *testing.T) {
	Convey("Given a quantity that shopspring would print without trailing zeros", t, func() {
		value, err := decimal.NewFromString("0.71")
		So(err, ShouldBeNil)

		Convey("When scaled to lot decimals for the SDK book", func() {
			scaled := scaledBookDecimal(value, 8)

			Convey("It keeps the fixed-point text Kraken's L3 checksum hashes", func() {
				So(scaled, ShouldNotBeNil)
				So(scaled.String(), ShouldEqual, "0.71000000")
			})
		})
	})
}
