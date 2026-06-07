package execution

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestEntryDeployFraction(t *testing.T) {
	Convey("Given deploy fraction inputs", t, func() {
		testconfig.Load(t)
		fraction, err := EntryDeployFraction(DeployFractionInput{
			PositionFraction: 1.0,
			ActFraction:      0.25,
			Regime:           types.RegimeTrending,
		})

		So(err, ShouldBeNil)
		So(fraction, ShouldEqual, 0.25)
	})
}

func TestEntrySlotSpend(t *testing.T) {
	Convey("Given a €200 account and 25% deploy", t, func() {
		slot := EntrySlotSpend(200, 0.25, 0.0026, 200)

		So(slot, ShouldAlmostEqual, 49.87, 0.01)
	})
}
