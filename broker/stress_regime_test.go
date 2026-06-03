package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestSymbolStressDeskRegimeForStress(t *testing.T) {
	Convey("Given fluid turbulence readings", t, func() {
		stress := SymbolStress{
			FluidCategory: perspectives.CategoryTurbulent,
			FluidSNR:      0.8,
		}

		Convey("It should restrict discretionary entries", func() {
			So(stress.DeskRegimeForStress(), ShouldEqual, DeskRegimeRestricted)
		})
	})

	Convey("Given calm fluid readings", t, func() {
		stress := SymbolStress{
			FluidCategory: perspectives.CategoryLaminar,
		}

		Convey("It should keep the desk in normal mode", func() {
			So(stress.DeskRegimeForStress(), ShouldEqual, DeskRegimeNormal)
		})
	})
}

func TestSymbolStressEntrySlippageCapBps(t *testing.T) {
	Convey("Given hostile toxicity stress", t, func() {
		stress := SymbolStress{
			ToxicityCategory: perspectives.CategoryToxicBluff,
			ToxicitySNR:      1,
		}

		Convey("It should tighten the configured slippage ceiling", func() {
			So(stress.EntrySlippageCapBps(50), ShouldAlmostEqual, 25, 1e-9)
		})
	})
}

func TestSymbolStressRejectsDiscretionaryEntry(t *testing.T) {
	Convey("Given decisive toxic bluff stress", t, func() {
		stress := SymbolStress{
			ToxicityCategory: perspectives.CategoryToxicBluff,
			ToxicitySNR:      1.2,
		}

		Convey("It should block discretionary entries", func() {
			So(stress.RejectsDiscretionaryEntry(), ShouldBeTrue)
		})
	})
}

func BenchmarkSymbolStressEntrySlippageCapBps(b *testing.B) {
	stress := SymbolStress{
		ToxicityCategory: perspectives.CategoryToxicBluff,
		ToxicitySNR:      1,
	}

	for b.Loop() {
		_ = stress.EntrySlippageCapBps(50)
	}
}
