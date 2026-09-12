package statistic_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/statistic"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestKish(t *testing.T) {
	Convey("Given equal weights", t, func() {
		kish := statistic.NewKish()
		out := tests.CollectSeq[float64](kish.Next(transport.NewValues(1.0, 1.0, 1.0, 1.0).Next(nil)))

		Convey("effective sample size equals the count", func() {
			So(kish.Error(), ShouldBeNil)
			So(out[len(out)-1], ShouldEqual, 4)
		})

		Convey("maturity follows 1 - 1/N_eff", func() {
			maturity := statistic.NewKishMaturity()
			matOut := tests.CollectSeq[float64](maturity.Next(transport.NewValues(out[len(out)-1]).Next(nil)))
			So(matOut[0], ShouldAlmostEqual, 0.75, 1e-12)

			matSingle := tests.CollectSeq[float64](maturity.Next(transport.NewValues(1.0).Next(nil)))
			So(matSingle[0], ShouldEqual, 0)

			matZero := tests.CollectSeq[float64](maturity.Next(transport.NewValues(0.0).Next(nil)))
			So(matZero[0], ShouldEqual, 0)
		})

		Convey("concentrated weights have lower effective support", func() {
			k := statistic.NewKish()
			res := tests.CollectSeq[float64](k.Next(transport.NewValues(1.0, 0.0, 0.0, 0.0).Next(nil)))
			So(res[len(res)-1], ShouldEqual, 1)
		})

		Convey("non-uniform weights yield the hand-calculated N_eff and maturity", func() {
			k := statistic.NewKish()
			res := tests.CollectSeq[float64](k.Next(transport.NewValues(1.0, 0.5, 0.25).Next(nil)))
			nEff := res[len(res)-1]
			So(nEff, ShouldAlmostEqual, 2.3333333333333335, 1e-12)

			maturity := statistic.NewKishMaturity()
			matOut := tests.CollectSeq[float64](maturity.Next(transport.NewValues(nEff).Next(nil)))
			So(matOut[0], ShouldAlmostEqual, 0.5714285714285714, 1e-12)
		})
	})
}
