package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestSymbolStressEntryExposureScale(t *testing.T) {
	Convey("Given fluid turbulence readings", t, func() {
		stress := SymbolStress{
			FluidCategory: types.CategoryTurbulent,
			FluidSNR:      0.8,
		}

		Convey("It should scale entries continuously", func() {
			So(stress.EntryExposureScale(), ShouldAlmostEqual, 1/1.8, 1e-9)
			So(stress.EntryQuantity(9), ShouldAlmostEqual, 5, 1e-9)
		})
	})

	Convey("Given calm fluid readings", t, func() {
		stress := SymbolStress{
			FluidCategory: types.CategoryLaminar,
		}

		Convey("It should leave entries unscaled", func() {
			So(stress.EntryExposureScale(), ShouldEqual, 1)
			So(stress.EntryQuantity(3), ShouldEqual, 3)
		})
	})

	Convey("Given zero requested quantity", t, func() {
		stress := SymbolStress{
			FluidCategory: types.CategoryTurbulent,
			FluidSNR:      0.8,
		}

		Convey("It should leave zero quantity untouched", func() {
			So(stress.EntryQuantity(0), ShouldEqual, 0)
		})
	})
}

func BenchmarkSymbolStressEntryExposureScale(b *testing.B) {
	stress := SymbolStress{
		ToxicityCategory: types.CategoryToxicBluff,
		ToxicitySNR:      1,
	}

	for b.Loop() {
		_ = stress.EntryExposureScale()
	}
}
