package statistic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestBranching(t *testing.T) {
	Convey("Given branching parameters", t, func() {
		params := types.NewMap[string, types.Value[float64]]()
		params.Put("alpha_aa", types.NewValue(1.0))
		params.Put("alpha_ab", types.NewValue(0.5))
		params.Put("alpha_ba", types.NewValue(0.5))
		params.Put("alpha_bb", types.NewValue(1.0))
		params.Put("beta", types.NewValue(2.0))

		branching := NewBranching(types.NewInput(types.NewValue(params)))

		Convey("Read should compute spectral radius and offspring", func() {
			branching.Write(types.NewInput(types.NewValue(params)))
			out := branching.Read()
			So(out.Error(), ShouldBeBlank)

			res := out.Project().Read()
			sr, ok := res.Get("spectral_radius")
			So(ok, ShouldBeTrue)
			So(sr.Read(), ShouldAlmostEqual, 0.75, 1e-9)

			offAA, ok := res.Get("offspring_aa")
			So(ok, ShouldBeTrue)
			So(offAA.Read(), ShouldAlmostEqual, 0.5, 1e-9)
		})

		Convey("Close should succeed", func() {
			So(branching.Close(), ShouldBeNil)
		})
	})
}
