package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/system"
)

func TestIgnitionHot(t *testing.T) {
	Convey("Setup", t, func() {
		sel := system.Cfg.Selection

		Convey("When reading is nil", func() {
			Convey("ignitionHot is false", func() {
				So(ignitionHot(nil), ShouldBeFalse)
			})
		})

		Convey("When precursor crosses the floor", func() {
			reading := &equation.IgnitionOutput{
				Precursor: sel.IgnitionPrecursorMin,
			}

			Convey("ignitionHot is true", func() {
				So(ignitionHot(reading), ShouldBeTrue)
			})
		})

		Convey("When all signals stay below floors", func() {
			reading := &equation.IgnitionOutput{
				Precursor:   sel.IgnitionPrecursorMin - 0.1,
				Compression: sel.IgnitionCompressionMin - 0.1,
				Ignition:    sel.IgnitionIgnitionMin - 0.1,
			}

			Convey("ignitionHot is false", func() {
				So(ignitionHot(reading), ShouldBeFalse)
			})
		})
	})
}
