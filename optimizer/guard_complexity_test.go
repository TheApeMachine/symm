package optimizer

import (
	"context"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestGateComplexityWeight(t *testing.T) {
	convey.Convey("Given gate pass rates on the profile", t, func() {
		profile := Profile{ctx: context.Background()}

		for index := range 100 {
			snr := 0.01

			if index >= 50 {
				snr = 2
			}

			profile.Add(perspectives.Measurement{
				Category: perspectives.CategoryLaminar,
				SNR:      snr,
			})
		}

		profile.PrepareCache()
		selective := perspectives.Branch{
			Category:  perspectives.CategoryLaminar,
			Condition: perspectives.ConditionIsGreaterThanOrEqual,
			Unit:      perspectives.UnitSNR,
			Value:     1,
			ValueSet:  true,
		}
		extreme := perspectives.Branch{
			Category:  perspectives.CategoryLaminar,
			Condition: perspectives.ConditionIsGreaterThanOrEqual,
			Unit:      perspectives.UnitSNR,
			Value:     0.01,
			ValueSet:  true,
		}

		convey.Convey("It should waive penalty for balanced selective gates", func() {
			convey.So(gateComplexityWeight(&profile, selective), convey.ShouldEqual, 0)
		})

		convey.Convey("It should amplify penalty for extreme pass rates", func() {
			convey.So(gateComplexityWeight(&profile, extreme), convey.ShouldBeGreaterThan, 1)
		})
	})
}

func TestOverfitGuardAdaptiveComplexityPenalty(t *testing.T) {
	convey.Convey("Given equal profit with selective shallow vs extreme deep gates", t, func() {
		profile := Profile{ctx: context.Background()}

		for index := range 100 {
			snr := 0.01

			if index >= 50 {
				snr = 2
			}

			profile.Add(perspectives.Measurement{
				Category: perspectives.CategoryLaminar,
				SNR:      snr,
			})
			profile.Add(perspectives.Measurement{
				Category: perspectives.CategoryExhaustion,
				SNR:      snr,
			})
		}

		profile.PrepareCache()
		guard := NewOverfitGuard(context.Background(), GuardOptions{
			ComplexityPenalty: 0.05,
		}, ReplayTape{}, &profile)
		shallow := perspectives.BranchList{{
			Category:  perspectives.CategoryLaminar,
			Condition: perspectives.ConditionIsGreaterThanOrEqual,
			Unit:      perspectives.UnitSNR,
			Value:     1,
			ValueSet:  true,
		}}
		deep := perspectives.BranchList{{
			Category:  perspectives.CategoryLaminar,
			Condition: perspectives.ConditionIsGreaterThanOrEqual,
			Unit:      perspectives.UnitSNR,
			Value:     0.01,
			ValueSet:  true,
			Branches: []perspectives.Branch{{
				Category:  perspectives.CategoryExhaustion,
				Condition: perspectives.ConditionIsGreaterThanOrEqual,
				Unit:      perspectives.UnitSNR,
				Value:     0.01,
				ValueSet:  true,
			}},
		}}

		convey.Convey("It should prefer the shallower selective tree", func() {
			shallowScore := guard.AdjustedScore(0.10, shallow)
			deepScore := guard.AdjustedScore(0.10, deep)
			convey.So(shallowScore, convey.ShouldBeGreaterThan, deepScore)
		})
	})
}

func BenchmarkAdaptiveComplexityPenalty(b *testing.B) {
	profile := Profile{ctx: context.Background()}

	for index := range 200 {
		profile.Add(perspectives.Measurement{
			Category: perspectives.CategoryLaminar,
			SNR:      float64(index % 5),
		})
	}

	profile.PrepareCache()
	guard := NewOverfitGuard(context.Background(), GuardOptions{
		ComplexityPenalty: 0.05,
	}, ReplayTape{}, &profile)
	branches := perspectives.BranchList{{
		Category:  perspectives.CategoryLaminar,
		Condition: perspectives.ConditionIsGreaterThanOrEqual,
		Unit:      perspectives.UnitSNR,
		Value:     1,
		ValueSet:  true,
		Branches: []perspectives.Branch{{
			Category:  perspectives.CategoryExhaustion,
			Condition: perspectives.ConditionIsGreaterThanOrEqual,
			Unit:      perspectives.UnitSNR,
			Value:     1,
			ValueSet:  true,
		}},
	}}

	b.ReportAllocs()

	for b.Loop() {
		_ = guard.adaptiveComplexityPenalty(branches)
	}
}
