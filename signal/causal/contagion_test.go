package causal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestContagionInsufficientHistory(t *testing.T) {
	Convey("Given symbols without enough HY intervals", t, func() {
		signal := &Signal{}
		signal.state("BTC/EUR")
		signal.state("ETH/EUR")

		Convey("It should report zero coupling", func() {
			So(signal.contagion(), ShouldEqual, 0)
		})
	})
}

func TestContagionWindowDefaults(t *testing.T) {
	Convey("Given default contagion helpers", t, func() {
		Convey("They should return positive defaults", func() {
			So(contagionWindow(), ShouldBeGreaterThan, 0)
			So(contagionMinSamples(), ShouldBeGreaterThan, 0)
		})
	})
}
