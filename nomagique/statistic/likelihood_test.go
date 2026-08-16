package statistic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestLikelihood(t *testing.T) {
	Convey("Given log likelihood parameters", t, func() {
		params := types.NewMap[string, types.Value[float64]]()
		params.Put("ll_hawkes", types.NewValue(-120.5))
		params.Put("ll_poisson", types.NewValue(-150.0))
		params.Put("ll_self", types.NewValue(-135.0))

		likelihood := NewLikelihood(types.NewInput(types.NewValue(params)))

		Convey("Read should compute deltas against Poisson and self-only baselines", func() {
			likelihood.Write(types.NewInput(types.NewValue(params)))
			out := likelihood.Read()
			So(out.Error(), ShouldBeBlank)

			res := out.Project().Read()
			deltaPoisson, ok := res.Get("ll_delta_poisson")
			So(ok, ShouldBeTrue)
			So(deltaPoisson.Read(), ShouldAlmostEqual, 29.5, 1e-9)

			deltaSelf, ok := res.Get("ll_delta_self")
			So(ok, ShouldBeTrue)
			So(deltaSelf.Read(), ShouldAlmostEqual, 14.5, 1e-9)
		})

		Convey("Close should succeed", func() {
			So(likelihood.Close(), ShouldBeNil)
		})
	})
}
