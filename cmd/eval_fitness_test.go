package cmd

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTuneFitness(t *testing.T) {
	Convey("Given wallet score and missed gate regret", t, func() {
		fitness := TuneFitness(12, 3)

		Convey("It should subtract missed forward EUR from score", func() {
			So(fitness, ShouldEqual, 9)
		})
	})

	Convey("Given zero missed regret", t, func() {
		fitness := TuneFitness(-4, 0)

		Convey("It should equal score alone", func() {
			So(fitness, ShouldEqual, -4)
		})
	})
}
