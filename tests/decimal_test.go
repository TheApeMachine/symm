package tests

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDecimal(t *testing.T) {
	Convey("Given a decimal string", t, func() {
		Convey("When Decimal is called", func() {
			parsed := Decimal(t, "0.5666")

			Convey("Then it returns the parsed value", func() {
				So(parsed.String(), ShouldEqual, "0.5666")
			})
		})
	})
}
