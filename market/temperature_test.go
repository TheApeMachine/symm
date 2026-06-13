package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/telemetry"
)

func TestMacroTemperature(t *testing.T) {
	testconfig.Load(t)

	Convey("Given a live surprise index", t, func() {
		telemetry.SharedSurpriseIndex().Record("fluid", 2, 1)

		temperature, ready := MacroTemperature()

		Convey("It should derive a bounded macro temperature", func() {
			So(ready, ShouldBeTrue)
			So(temperature, ShouldBeGreaterThan, 0)
			So(temperature, ShouldBeLessThanOrEqualTo, 1)
		})
	})
}
