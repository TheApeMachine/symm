package optimizer

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
)

func TestDefaultTunables(t *testing.T) {
	testconfig.Load(t)

	Convey("Given default tunables from config", t, func() {
		tunables := DefaultTunables()

		Convey("It should mirror viper signal defaults", func() {
			So(tunables.SpoofWeightedThreshold, ShouldBeGreaterThan, 0)
			So(tunables.ContagionBreak, ShouldBeGreaterThan, 0)
			So(len(tunables.TunableSpecs()), ShouldBeGreaterThan, 0)
		})

		Convey("It should round-trip through Apply and Clone", func() {
			tunables.EntryEdgeMultiple = 2
			tunables.Apply()
			clone := tunables.Clone()

			So(clone.EntryEdgeMultiple, ShouldEqual, 2)
		})
	})
}
