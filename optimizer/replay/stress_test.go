package replay

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestExecutionStressMultiplier(t *testing.T) {
	convey.Convey("Given turbulent snapshot readings", t, func() {
		snapshots := []perspectives.Measurement{
			{Category: perspectives.CategoryTurbulent, SNR: 2},
			{Category: perspectives.CategoryLaminar, SNR: 0.5},
		}

		multiplier := executionStressMultiplier(snapshots)

		convey.Convey("It should expand slippage above baseline", func() {
			convey.So(multiplier, convey.ShouldBeGreaterThan, 1)
		})
	})

	convey.Convey("Given only laminar readings", t, func() {
		snapshots := []perspectives.Measurement{
			{Category: perspectives.CategoryLaminar, SNR: 2},
		}

		convey.Convey("It should leave slippage unchanged", func() {
			convey.So(executionStressMultiplier(snapshots), convey.ShouldEqual, 1)
		})
	})
}

func TestReplaySimulationExecutionLatency(t *testing.T) {
	convey.Convey("Given timed entry and exit rows with execution latency", t, func() {
		ctx := context.Background()
		base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
		rows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      2, Last: 100, At: base,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      2, Last: 101, At: base.Add(100 * time.Millisecond),
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion,
				SNR:      2, Last: 102, At: base.Add(200 * time.Millisecond),
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
		immediate := ReplayCosts{
			MakerFeePct:            0.0016,
			TakerFeePct:            0.0026,
			SlippagePct:            0.0005,
			ExecutionStressEnabled: false,
		}
		delayed := immediate
		delayed.ExecutionLatency = 150 * time.Millisecond
		delayed.ExecutionStressEnabled = true

		immediateScore := NewReplaySimulationWithCosts(ctx, branches, rows, immediate).Score()
		delayedScore := NewReplaySimulationWithCosts(ctx, branches, rows, delayed).Score()

		convey.Convey("It should defer fills until latency elapses", func() {
			convey.So(delayedScore, convey.ShouldBeLessThan, immediateScore)
		})
	})
}

func TestReplaySimulationStressSlippage(t *testing.T) {
	convey.Convey("Given turbulent snapshot context during entry", t, func() {
		ctx := context.Background()
		calmRows := []perspectives.Measurement{
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      2, Last: 100, SpreadBPS: 10,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion,
				SNR:      2, Last: 100.20, SpreadBPS: 10,
			},
		}
		stressRows := []perspectives.Measurement{
			{
				Source:   perspectives.SourceCausal,
				Category: perspectives.CategoryTurbulent,
				SNR:      3,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceFluid,
				Category: perspectives.CategoryLaminar,
				SNR:      3, Last: 100, SpreadBPS: 10,
			},
			{
				Symbol: "BTC/EUR", Source: perspectives.SourceExhaustion,
				Category: perspectives.CategoryExhaustion,
				SNR:      2, Last: 100.20, SpreadBPS: 10,
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
		costs := ReplayCosts{
			MakerFeePct:            0.0016,
			TakerFeePct:            0.0026,
			SlippagePct:            0.0005,
			ExecutionStressEnabled: true,
		}

		calmScore := NewReplaySimulationWithCosts(ctx, branches, calmRows, costs).Score()
		stressScore := NewReplaySimulationWithCosts(ctx, branches, stressRows, costs).Score()

		convey.Convey("It should charge more drag during stress categories", func() {
			convey.So(stressScore, convey.ShouldBeLessThan, calmScore)
		})
	})
}

func BenchmarkExecutionStressMultiplier(b *testing.B) {
	snapshots := []perspectives.Measurement{
		{Category: perspectives.CategoryTurbulent, SNR: 2},
		{Category: perspectives.CategoryLiquidityShock, SNR: 1.5},
		{Category: perspectives.CategoryLaminar, SNR: 0.5},
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = executionStressMultiplier(snapshots)
	}
}

func TestReplaySimulationUsesStoredLatencyProfile(t *testing.T) {
	convey.Convey("Given a persisted exchange latency profile", t, func() {
		path := filepath.Join(t.TempDir(), "network_latency.json")
		viper.Set("trading.paper.latency_profile", path)

		defer viper.Set("trading.paper.latency_profile", "")

		costs := ReplayCosts{ExecutionStressEnabled: true}
		latency := costs.effectiveExecutionLatency(nil, ReplayTape{})

		convey.Convey("It should score with stored RTT without sleeping", func() {
			convey.So(latency, convey.ShouldEqual, 95*time.Millisecond)
		})
	})
}

func BenchmarkReplayMeasurementsPool(b *testing.B) {
	measurement := perspectives.Measurement{
		Symbol:   "BTC/EUR",
		Source:   perspectives.SourceFluid,
		Category: perspectives.CategoryLaminar,
		SNR:      2,
		Last:     100,
	}

	b.ReportAllocs()

	for b.Loop() {
		measurements := acquireReplayMeasurements()
		measurements.Add(measurement)
		_ = measurements.Snapshot("BTC/EUR")
		releaseReplayMeasurements(measurements)
	}
}
