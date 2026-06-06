package perspectives

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestWindowReason(t *testing.T) {
	Convey("Given a pump window and an open long", t, func() {
		// oldest -> newest; NewWindowReason indexes newest-first.
		snapshots := []types.Measurement{
			{Category: types.CategoryCoiledCompression, SNR: 1.0, Last: 100},
			{Category: types.CategoryCoiledCompression, SNR: 1.1, Last: 101},
			{Category: types.CategoryVerticalIgnition, SNR: 0.5, Last: 102},
			{Category: types.CategoryVerticalIgnition, SNR: 2.0, Last: 105},
		}

		holding := PositionState{Holding: true, EntryPrice: 100, Peak: 105, Last: 105}
		ctx := NewWindowReason(snapshots, types.RegimeTrending, holding)

		Convey("Signals read newest-first per category", func() {
			snr, ok := ctx.Signal(types.CategoryVerticalIgnition, reasoning.UnitSNR, 0)
			So(ok, ShouldBeTrue)
			So(snr, ShouldEqual, 2.0)
			prev, _ := ctx.Signal(types.CategoryVerticalIgnition, reasoning.UnitSNR, 1)
			So(prev, ShouldEqual, 0.5)
		})

		Convey("Price reads distinct changes newest-first", func() {
			now, _ := ctx.Scalar(SubjectPrice, reasoning.UnitNone, 0)
			then, _ := ctx.Scalar(SubjectPrice, reasoning.UnitNone, 3)
			So(now, ShouldEqual, 105)
			So(then, ShouldEqual, 100) // +5% over the window
		})

		Convey("The lifecycle reflects a move that is running", func() {
			So(ctx.Lifecycle(types.ObservationHolding), ShouldBeTrue)
			So(ctx.Lifecycle(types.ObservationHasContinued), ShouldBeTrue) // peak 105 >= entry*1.01
			So(ctx.Lifecycle(types.ObservationHasStarted), ShouldBeFalse)  // it has already continued
			So(ctx.Lifecycle(types.ObservationHasEnded), ShouldBeFalse)    // last 105 > peak*0.99
		})

		Convey("A retrace from the peak flips has_ended", func() {
			retraced := NewWindowReason(snapshots, types.RegimeTrending,
				PositionState{Holding: true, EntryPrice: 100, Peak: 105, Last: 103})
			So(retraced.Lifecycle(types.ObservationHasEnded), ShouldBeTrue) // 103 <= 105*0.99
		})

		Convey("Predicates resolve correctly through the real context", func() {
			// price rose >= 2% over the last 3 changes
			So(holds(Predicate{
				Subject: SubjectPrice, Unit: reasoning.UnitPercentage, Ago: 3, Op: ComparisonRoseBy, Value: 2.0,
			}, ctx), ShouldBeTrue)

			// metric-to-metric: ignition now (2.0) above compression now (1.1)
			So(holds(Predicate{
				Subject: SubjectSignal, Category: types.CategoryVerticalIgnition, Unit: reasoning.UnitSNR, Op: ComparisonAbove,
				Versus: &Operand{Subject: SubjectSignal, Category: types.CategoryCoiledCompression, Unit: reasoning.UnitSNR},
			}, ctx), ShouldBeTrue)

			// ignition crossed up through 1.5 within the last reading
			So(holds(Predicate{
				Subject: SubjectSignal, Category: types.CategoryVerticalIgnition, Unit: reasoning.UnitSNR,
				Ago: 1, Op: ComparisonCrossedUp, Value: 1.5,
			}, ctx), ShouldBeTrue)
		})

		Convey("Reset clears categories that disappear from the next window", func() {
			ctx.Reset([]types.Measurement{{Category: types.CategoryCoiledCompression, SNR: 1.2, Last: 106}},
				types.RegimeChoppy, holding)

			_, ok := ctx.Signal(types.CategoryVerticalIgnition, reasoning.UnitSNR, 0)
			So(ok, ShouldBeFalse)

			snr, ok := ctx.Signal(types.CategoryCoiledCompression, reasoning.UnitSNR, 0)
			So(ok, ShouldBeTrue)
			So(snr, ShouldEqual, 1.2)
		})

		Convey("Volume reads distinct notional changes newest-first", func() {
			volumeSnapshots := []types.Measurement{
				{Volume: 100, Last: 10},
				{Volume: 110, Last: 10},
				{Volume: 130, Last: 10},
			}
			volumeCtx := NewWindowReason(volumeSnapshots, types.RegimeBullish, PositionState{})

			now, okNow := volumeCtx.Scalar(SubjectVolume, reasoning.UnitNone, 0)
			then, okThen := volumeCtx.Scalar(SubjectVolume, reasoning.UnitNone, 2)

			So(okNow, ShouldBeTrue)
			So(okThen, ShouldBeTrue)
			So(now, ShouldEqual, 130)
			So(then, ShouldEqual, 100)

			So(holds(Predicate{
				Subject: SubjectVolume, Unit: reasoning.UnitPercentage, Ago: 2, Op: ComparisonRoseBy, Value: 30.0,
			}, volumeCtx), ShouldBeTrue)
		})
	})
}

func BenchmarkNewWindowReason(b *testing.B) {
	snapshots := []types.Measurement{
		{Category: types.CategoryCoiledCompression, SNR: 1.0, Last: 100},
		{Category: types.CategoryCoiledCompression, SNR: 1.1, Last: 101},
		{Category: types.CategoryVerticalIgnition, SNR: 0.5, Last: 102},
		{Category: types.CategoryVerticalIgnition, SNR: 2.0, Last: 105},
	}
	holding := PositionState{Holding: true, EntryPrice: 100, Peak: 105, Last: 105}

	for b.Loop() {
		_ = NewWindowReason(snapshots, types.RegimeTrending, holding)
	}
}

func BenchmarkWindowReasonReset(b *testing.B) {
	snapshots := []types.Measurement{
		{Category: types.CategoryCoiledCompression, SNR: 1.0, Last: 100},
		{Category: types.CategoryCoiledCompression, SNR: 1.1, Last: 101},
		{Category: types.CategoryVerticalIgnition, SNR: 0.5, Last: 102},
		{Category: types.CategoryVerticalIgnition, SNR: 2.0, Last: 105},
	}
	holding := PositionState{Holding: true, EntryPrice: 100, Peak: 105, Last: 105}
	reason := &WindowReason{}

	for b.Loop() {
		_ = reason.Reset(snapshots, types.RegimeTrending, holding)
	}
}
