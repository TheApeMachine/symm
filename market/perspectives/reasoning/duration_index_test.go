package reasoning

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestWindowReasonWithinLookback(t *testing.T) {
	Convey("Given measurements stamped on a wall clock", t, func() {
		base := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
		snapshots := []types.Measurement{
			{At: base, Category: types.CategoryVerticalIgnition, SNR: 1.0, Last: 100},
			{At: base.Add(200 * time.Millisecond), Category: types.CategoryVerticalIgnition, SNR: 1.5, Last: 101},
			{At: base.Add(400 * time.Millisecond), Category: types.CategoryVerticalIgnition, SNR: 2.0, Last: 102},
			{At: base.Add(600 * time.Millisecond), Category: types.CategoryVerticalIgnition, SNR: 3.0, Last: 105},
		}

		position := PositionState{
			Holding: true,
			Last:    105,
			Now:     base.Add(600 * time.Millisecond),
		}
		ctx := NewWindowReason(snapshots, types.RegimeTrending, position)

		Convey("within resolves to the reading at or before the cutoff", func() {
			then, ok := ctx.Signal(
				types.CategoryVerticalIgnition,
				UnitSNR,
				Lookback{Within: 500 * time.Millisecond},
			)

			So(ok, ShouldBeTrue)
			So(then, ShouldEqual, 1.0)

			So(holds(Predicate{
				Subject: SubjectSignal,
				Category: types.CategoryVerticalIgnition,
				Unit:     UnitSNR,
				Within:   YAMLDuration(500 * time.Millisecond),
				Op:       ComparisonRoseBy,
				Value:    1.0,
			}, ctx), ShouldBeTrue)
		})

		Convey("repeated within lookups reuse the cached tail index", func() {
			first, okFirst := ctx.resolveSignalIndex(
				types.CategoryVerticalIgnition,
				Lookback{Within: 500 * time.Millisecond},
			)
			second, okSecond := ctx.resolveSignalIndex(
				types.CategoryVerticalIgnition,
				Lookback{Within: 500 * time.Millisecond},
			)

			So(okFirst, ShouldBeTrue)
			So(okSecond, ShouldBeTrue)
			So(first, ShouldEqual, second)
		})
	})
}

func BenchmarkWindowReasonWithinLookup(b *testing.B) {
	base := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	snapshots := make([]types.Measurement, 256)

	for index := range snapshots {
		snapshots[index] = types.Measurement{
			At:       base.Add(time.Duration(index) * time.Millisecond),
			Category: types.CategoryVerticalIgnition,
			SNR:      float64(index),
			Last:     100 + float64(index),
		}
	}

	position := PositionState{Now: snapshots[len(snapshots)-1].At}
	reason := NewWindowReason(snapshots, types.RegimeTrending, position)
	lookback := Lookback{Within: 200 * time.Millisecond}

	b.ReportAllocs()

	for b.Loop() {
		_, _ = reason.Signal(types.CategoryVerticalIgnition, UnitSNR, lookback)
	}
}
