package perspectives

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWindowReason(t *testing.T) {
	Convey("Given a pump window and an open long", t, func() {
		// oldest -> newest; NewWindowReason indexes newest-first.
		snapshots := []Measurement{
			{Category: CategoryCoiledCompression, SNR: 1.0, Last: 100},
			{Category: CategoryCoiledCompression, SNR: 1.1, Last: 101},
			{Category: CategoryVerticalIgnition, SNR: 0.5, Last: 102},
			{Category: CategoryVerticalIgnition, SNR: 2.0, Last: 105},
		}

		holding := PositionState{Holding: true, EntryPrice: 100, Peak: 105, Last: 105}
		ctx := NewWindowReason(snapshots, RegimeTrending, holding)

		Convey("Signals read newest-first per category", func() {
			snr, ok := ctx.Signal(CategoryVerticalIgnition, UnitSNR, 0)
			So(ok, ShouldBeTrue)
			So(snr, ShouldEqual, 2.0)
			prev, _ := ctx.Signal(CategoryVerticalIgnition, UnitSNR, 1)
			So(prev, ShouldEqual, 0.5)
		})

		Convey("Price reads distinct changes newest-first", func() {
			now, _ := ctx.Scalar(SubjectPrice, UnitNone, 0)
			then, _ := ctx.Scalar(SubjectPrice, UnitNone, 3)
			So(now, ShouldEqual, 105)
			So(then, ShouldEqual, 100) // +5% over the window
		})

		Convey("The lifecycle reflects a move that is running", func() {
			So(ctx.Lifecycle(ObservationHolding), ShouldBeTrue)
			So(ctx.Lifecycle(ObservationHasContinued), ShouldBeTrue) // peak 105 >= entry*1.01
			So(ctx.Lifecycle(ObservationHasStarted), ShouldBeFalse)  // it has already continued
			So(ctx.Lifecycle(ObservationHasEnded), ShouldBeFalse)    // last 105 > peak*0.99
		})

		Convey("A retrace from the peak flips has_ended", func() {
			retraced := NewWindowReason(snapshots, RegimeTrending,
				PositionState{Holding: true, EntryPrice: 100, Peak: 105, Last: 103})
			So(retraced.Lifecycle(ObservationHasEnded), ShouldBeTrue) // 103 <= 105*0.99
		})

		Convey("Predicates resolve correctly through the real context", func() {
			// price rose >= 2% over the last 3 changes
			So(holds(Predicate{
				Subject: SubjectPrice, Unit: UnitPercentage, Ago: 3, Op: ComparisonRoseBy, Value: 2.0,
			}, ctx), ShouldBeTrue)

			// metric-to-metric: ignition now (2.0) above compression now (1.1)
			So(holds(Predicate{
				Subject: SubjectSignal, Category: CategoryVerticalIgnition, Unit: UnitSNR, Op: ComparisonAbove,
				Versus: &Operand{Subject: SubjectSignal, Category: CategoryCoiledCompression, Unit: UnitSNR},
			}, ctx), ShouldBeTrue)

			// ignition crossed up through 1.5 within the last reading
			So(holds(Predicate{
				Subject: SubjectSignal, Category: CategoryVerticalIgnition, Unit: UnitSNR,
				Ago: 1, Op: ComparisonCrossedUp, Value: 1.5,
			}, ctx), ShouldBeTrue)
		})

		Convey("Reset clears categories that disappear from the next window", func() {
			ctx.Reset([]Measurement{{Category: CategoryCoiledCompression, SNR: 1.2, Last: 106}},
				RegimeChoppy, holding)

			_, ok := ctx.Signal(CategoryVerticalIgnition, UnitSNR, 0)
			So(ok, ShouldBeFalse)

			snr, ok := ctx.Signal(CategoryCoiledCompression, UnitSNR, 0)
			So(ok, ShouldBeTrue)
			So(snr, ShouldEqual, 1.2)
		})
	})
}

func BenchmarkNewWindowReason(b *testing.B) {
	snapshots := []Measurement{
		{Category: CategoryCoiledCompression, SNR: 1.0, Last: 100},
		{Category: CategoryCoiledCompression, SNR: 1.1, Last: 101},
		{Category: CategoryVerticalIgnition, SNR: 0.5, Last: 102},
		{Category: CategoryVerticalIgnition, SNR: 2.0, Last: 105},
	}
	holding := PositionState{Holding: true, EntryPrice: 100, Peak: 105, Last: 105}

	for b.Loop() {
		_ = NewWindowReason(snapshots, RegimeTrending, holding)
	}
}

func BenchmarkWindowReasonReset(b *testing.B) {
	snapshots := []Measurement{
		{Category: CategoryCoiledCompression, SNR: 1.0, Last: 100},
		{Category: CategoryCoiledCompression, SNR: 1.1, Last: 101},
		{Category: CategoryVerticalIgnition, SNR: 0.5, Last: 102},
		{Category: CategoryVerticalIgnition, SNR: 2.0, Last: 105},
	}
	holding := PositionState{Holding: true, EntryPrice: 100, Peak: 105, Last: 105}
	reason := &WindowReason{}

	for b.Loop() {
		_ = reason.Reset(snapshots, RegimeTrending, holding)
	}
}
