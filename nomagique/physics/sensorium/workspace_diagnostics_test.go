//go:build (darwin && cgo) || (linux && cuda && cgo)

package sensorium

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestAccountPhase(t *testing.T) {
	Convey("Phase dissipation uses the forcing amplitude as its rounding scale", t, func() {
		fluid := remapWorkspace(t, 8, 1)
		ledger := fluid.phaseLedger.Float32Slice()
		ledger[5] = 1e-10

		Convey("Cancellation near quadrature retains signed roundoff", func() {
			ledger[1], ledger[2] = 1e-18, 8e-18
			So(fluid.accountPhase(), ShouldBeNil)
			So(fluid.health.Sources.PhaseDissipation, ShouldBeLessThan, 0)
		})

		Convey("An increase exceeding forcing roundoff is rejected", func() {
			ledger[1], ledger[2] = 0, 1e-11
			So(fluid.accountPhase(), ShouldNotBeNil)
		})
	})
}
