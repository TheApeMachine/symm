package replay

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestEntryDeployFraction(t *testing.T) {
	Convey("Given a node multiplier and choppy regime", t, func() {
		testconfig.Load(t)

		costs := ReplayCosts{PositionFraction: 0.1}
		act := reasoning.Act{Fraction: 2}
		snapshots := []types.Measurement{
			{Category: types.CategoryTurbulent, SNR: 2},
		}

		fraction, err := entryDeployFraction(costs, act, snapshots)
		So(err, ShouldBeNil)

		Convey("It should scale the base fraction by the node multiplier and regime scale", func() {
			So(fraction, ShouldAlmostEqual, 0.2, 1e-9)
		})
	})
}
