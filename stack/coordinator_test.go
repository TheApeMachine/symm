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
			Symbol: "BTC/USD",
			At:     time.Now().UTC(),
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricEventCount, types.SideNone): {
					Raw: 1,
				},
			},
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
			Symbol: "ETH/USD",
			At:     thesis.At,
			Metrics: map[string]types.MetricSample{
				types.MetricKey(types.MetricEventCount, types.SideNone): {
					Raw: 3,
				},
			},
		}})

		cut := types.NewImmutableCut(1, 7, thesis)

		Convey("It isolates the cut slice from subsequent Publish pointer replacements", func() {
			So(cut.Tick, ShouldEqual, int64(7))
			So(len(cut.Measurements), ShouldEqual, 1)

			// Publish replaces the slot pointer in the thesis — the cut's frozen
			// slice must still point to the original row with Raw=3.
			thesis.Publish(types.SourceHawkes, []*types.Measurement{{
				Source: types.SourceHawkes,
				Symbol: "ETH/USD",
				At:     thesis.At,
				Metrics: map[string]types.MetricSample{
					types.MetricKey(types.MetricEventCount, types.SideNone): {
						Raw: 99,
					},
				},
			}})

			So(
				cut.Measurements[0].Metrics[types.MetricKey(types.MetricEventCount, types.SideNone)].Raw,
				ShouldEqual,
				float64(3),
			)
		})
	})
}
