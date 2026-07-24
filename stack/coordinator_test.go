package stack

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestCutCoordinatorFinalize(t *testing.T) {
	Convey("Given a CutCoordinator on Hawkes cadence", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		thesis := types.NewThesis()
		thesis.Publish(types.SourceHawkes, []*types.Measurement{{
			Source: types.SourceHawkes,
			Metric: types.MetricEventCount,
			Symbol: "BTC/USD",
			Raw:    1,
			At:     time.Now().UTC(),
		}})

		coordinator := NewCutCoordinator(
			ctx, thesis, types.SourceHawkes, types.SourcePumpDump,
		)

		Convey("It freezes an ImmutableCut and advances tick", func() {
			frame := coordinator.onHawkes(thesis)
			typed, ok := frame.(*cutFrame)
			So(ok, ShouldBeTrue)
			So(typed.Cut, ShouldNotBeNil)
			So(typed.Cut.ID, ShouldBeGreaterThan, types.CutID(0))
			So(typed.Thesis.Tick, ShouldBeGreaterThan, int64(0))
			So(len(typed.Cut.Measurements), ShouldEqual, 1)
		})

		Convey("It preserves unapplied Enter decisions across finalize", func() {
			thesis.Decisions = []types.Decision{{
				Action: types.ActionEnter,
				Symbol: "BTC/USD",
			}}
			frame := coordinator.onHawkes(thesis)
			typed := frame.(*cutFrame)
			So(len(typed.Thesis.Decisions), ShouldEqual, 1)
			So(typed.Thesis.Decisions[0].Action, ShouldEqual, types.ActionEnter)
		})

		Reset(func() {
			_ = coordinator.Close()
		})
	})
}

func TestCutCoordinatorReport(t *testing.T) {
	Convey("Given an active cut", t, func() {
		ctx := context.Background()
		thesis := types.NewThesis()
		coordinator := NewCutCoordinator(ctx, thesis, types.SourceHawkes, types.SourceCVD)
		cutID := coordinator.begin(thesis)

		coordinator.Report(types.SignalResult{
			CutID:  cutID,
			Source: types.SourceCVD,
			Status: types.SignalReady,
		})

		Convey("It records the result for that CutID", func() {
			coordinator.barrier.mu.Lock()
			defer coordinator.barrier.mu.Unlock()
			So(coordinator.barrier.pending[cutID][types.SourceCVD].Status, ShouldEqual, types.SignalReady)
		})
	})
}

func BenchmarkCutCoordinatorFinalize(b *testing.B) {
	ctx := context.Background()
	thesis := types.NewThesis()
	coordinator := NewCutCoordinator(ctx, thesis, types.SourceHawkes)

	b.ReportAllocs()

	for b.Loop() {
		_ = coordinator.onHawkes(thesis)
	}
}

func TestImmutableCutClone(t *testing.T) {
	Convey("Given measurements on a thesis", t, func() {
		thesis := types.NewThesis()
		thesis.At = time.Now().UTC()
		thesis.Publish(types.SourceHawkes, []*types.Measurement{{
			Source: types.SourceHawkes,
			Metric: types.MetricEventCount,
			Symbol: "ETH/USD",
			Raw:    3,
			At:     thesis.At,
		}})

		cut := types.NewImmutableCut(1, 7, thesis)

		Convey("It deep-copies measurements", func() {
			So(cut.Tick, ShouldEqual, int64(7))
			So(len(cut.Measurements), ShouldEqual, 1)
			cut.Measurements[0].Symbol = "MUTATED"
			live := thesis.SnapshotMeasurements()
			So(live[0].Symbol, ShouldEqual, "ETH/USD")
		})
	})
}
