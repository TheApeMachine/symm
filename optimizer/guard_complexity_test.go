package optimizer

import (
	"context"
	"math"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestBinaryEntropyAndInformationGain(t *testing.T) {
	convey.Convey("Given balanced and separated win/loss splits", t, func() {
		balanced := GatePathStats{BeforeWins: 5, BeforeLoss: 5, AfterWins: 5, AfterLoss: 5}
		separated := GatePathStats{BeforeWins: 5, BeforeLoss: 5, AfterWins: 8, AfterLoss: 2}

		convey.Convey("It should report higher information gain for separated outcomes", func() {
			convey.So(binaryEntropyBits(5, 5), convey.ShouldAlmostEqual, 1, 0.001)
			convey.So(separated.InformationGainBits(), convey.ShouldBeGreaterThan, balanced.InformationGainBits())
		})
	})
}

func TestInformationGainSignificant(t *testing.T) {
	convey.Convey("Given enough separated trades", t, func() {
		stats := GatePathStats{
			BeforeWins: 10,
			BeforeLoss: 10,
			AfterWins:  16,
			AfterLoss:  4,
		}

		convey.Convey("It should waive complexity when gain clears the confidence bar", func() {
			convey.So(informationGainSignificant(stats, 4), convey.ShouldBeTrue)
		})
	})
}

func TestCollectGateReplayStats(t *testing.T) {
	convey.Convey("Given a replay tape with selective entry and exit", t, func() {
		ctx := context.Background()
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar, SNR: 2, Last: 100,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion, SNR: 2, Last: 110,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion, SNR: 2, Last: 105,
			},
		}
		branches := perspectives.BranchList{
			{
				Category:    perspectives.CategoryLaminar,
				Observation: perspectives.ObservationNotHolding,
				Condition:   perspectives.ConditionIsGreaterThanOrEqual,
				Unit:        perspectives.UnitSNR,
				Value:       1, ValueSet: true,
				Action: perspectives.Action{Type: perspectives.ActionLimit},
			},
			{
				Category:    perspectives.CategoryExhaustion,
				Observation: perspectives.ObservationHolding,
				Condition:   perspectives.ConditionIsGreaterThanOrEqual,
				Unit:        perspectives.UnitSNR,
				Value:       1, ValueSet: true,
				Action: perspectives.Action{Type: perspectives.ActionSettlePosition},
			},
		}
		tape := PrecompileTape(rows)
		collector := collectGateReplayStats(ctx, tape, branches)
		entryGate := branches[0]
		stats := collector.statsFor(entryGate)

		convey.Convey("It should derive tape pass rate and trade outcomes from replay", func() {
			convey.So(stats.TapeBefore, convey.ShouldBeGreaterThan, 0)
			convey.So(stats.TapePasses, convey.ShouldBeGreaterThan, 0)
			convey.So(stats.AfterWins+stats.AfterLoss, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestGateComplexityWeightUsesInformationGain(t *testing.T) {
	convey.Convey("Given a gate with significant replay information gain", t, func() {
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
		collector := &gateStatsCollector{
			slots: []gateStatsSlot{{
				key: gateStatsKey{
					category:  perspectives.CategoryLaminar,
					unit:      perspectives.UnitSNR,
					condition: perspectives.ConditionIsGreaterThanOrEqual,
					value:     1,
				},
			}},
			stats: []GatePathStats{{
				TapeBefore: 100,
				TapePasses: 50,
				BeforeWins: 10,
				BeforeLoss: 10,
				AfterWins:  16,
				AfterLoss:  4,
			}},
		}
		gate := perspectives.Branch{
			Category:  perspectives.CategoryLaminar,
			Condition: perspectives.ConditionIsGreaterThanOrEqual,
			Unit:      perspectives.UnitSNR,
			Value:     1,
			ValueSet:  true,
		}

		convey.Convey("It should waive penalty despite mid-range pass rate", func() {
			convey.So(
				gateComplexityWeight(&profile, collector, gate, 4),
				convey.ShouldEqual,
				0,
			)
		})
	})
}

func TestGateComplexityWeightAmplifiesExtremePassRate(t *testing.T) {
	convey.Convey("Given an extreme replay pass rate", t, func() {
		collector := &gateStatsCollector{
			slots: []gateStatsSlot{{
				key: gateStatsKey{
					category:  perspectives.CategoryLaminar,
					unit:      perspectives.UnitSNR,
					condition: perspectives.ConditionIsGreaterThanOrEqual,
					value:     0.01,
				},
			}},
			stats: []GatePathStats{{
				TapeBefore: 100,
				TapePasses: 99,
			}},
		}
		gate := perspectives.Branch{
			Category:  perspectives.CategoryLaminar,
			Condition: perspectives.ConditionIsGreaterThanOrEqual,
			Unit:      perspectives.UnitSNR,
			Value:     0.01,
			ValueSet:  true,
		}

		convey.Convey("It should amplify the complexity weight", func() {
			convey.So(gateComplexityWeight(nil, collector, gate, 4), convey.ShouldBeGreaterThan, 1)
		})
	})
}

func TestCausalStructuralRegimeUsesConditionSwitch(t *testing.T) {
	convey.Convey("Given a causal liquidity shock above the condition switch", t, func() {
		viper.Set("signals.causal.condition_switch", 100.0)
		snapshots := []perspectives.Measurement{{
			Source:   perspectives.SourceCausal,
			Category: perspectives.CategoryLiquidityShock,
			Strength: 150,
			SNR:      2,
		}}

		convey.Convey("It should classify the tick as liquidity panic", func() {
			regime, ok := causalStructuralRegime(snapshots)
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(regime, convey.ShouldEqual, StructuralRegimeLiquidityPanic)
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
	ctx := context.Background()
	rows := profile.Rows()
	tape := PrecompileTape(rows)
	guard := NewOverfitGuard(ctx, GuardOptions{
		ComplexityPenalty: 0.05,
	}, tape, &profile)
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

func BenchmarkCollectGateReplayStats(b *testing.B) {
	ctx := context.Background()
	rows := make([]perspectives.Measurement, 0, 200)

	for index := range 200 {
		rows = append(rows, perspectives.Measurement{
			Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
			Category: perspectives.CategoryLaminar,
			SNR:      float64(index % 5), Last: 100 + float64(index),
		})
	}

	rows = append(rows, perspectives.Measurement{
		Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
		Category: perspectives.CategoryExhaustion, SNR: 2, Last: 120,
	})
	branches := perspectives.BranchList{{
		Category:    perspectives.CategoryLaminar,
		Observation: perspectives.ObservationNotHolding,
		Condition:   perspectives.ConditionIsGreaterThanOrEqual,
		Unit:        perspectives.UnitSNR,
		Value:       1, ValueSet: true,
		Action: perspectives.Action{Type: perspectives.ActionLimit},
	}}
	tape := PrecompileTape(rows)

	b.ReportAllocs()

	for b.Loop() {
		_ = collectGateReplayStats(ctx, tape, branches)
	}
}

func BenchmarkBinaryEntropyBits(b *testing.B) {
	for b.Loop() {
		_ = binaryEntropyBits(40, 60)
	}
}

func init() {
	if math.Abs(viper.GetViper().GetFloat64("signals.causal.condition_switch")) <= 0 {
		viper.Set("signals.causal.condition_switch", 100.0)
	}
}
