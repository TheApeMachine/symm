package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestTemporalLedgerNestedHorizonResolution(t *testing.T) {
	Convey("Given a temporal ledger over nested cumulative directional targets", t, func() {
		head := func() *ResonanceManifold {
			return NewResonanceManifold([]int{2, 4, 2}, 1, 4, 0.05, ReadoutAll).(*ResonanceManifold)
		}

		// Each observation resolves, then issues a flat prediction so a
		// resolved ±1 target always carries non-zero error and every horizon
		// row gains scale evidence.
		observe := func(ledger *TemporalLedger, reference float64, callerStep int64) [9]float64 {
			data.Read[[9]float64](ledger.Next(data.NewValue(
				[3][]float64{{LedgerResolve, float64(callerStep), reference}, nil, nil},
			).Next(nil)))
			So(ledger.Error(), ShouldBeNil)

			reading := data.Read[[9]float64](ledger.Next(data.NewValue(
				[3][]float64{
					{LedgerIssue, float64(callerStep), reference, 1},
					make([]float64, 12),
					{0, 0, 0, 0},
				},
			).Next(nil)))
			So(ledger.Error(), ShouldBeNil)

			return reading
		}

		Convey("Every horizon of every row resolves against its own cumulative target", func() {
			manifold := head()
			ledger := NewTemporalLedger(4, manifold, NewDirectionalTarget(0.01)).(*TemporalLedger)
			var reading [9]float64

			for stepIndex := int64(1); stepIndex <= 10; stepIndex++ {
				reading = observe(ledger, 100+float64(stepIndex)*0.1, stepIndex)
			}

			// A row is fully supervised once four subsequent references have
			// arrived; ten steps fully resolve six rows (1-5, 2-6, 3-7, 4-8, 5-9, 6-10).
			So(reading[0], ShouldEqual, 6)

			// Horizon one and horizon four of the supervised rows must both
			// have resolved samples, so the per-horizon head is fully warm.
			So(manifold.taskScaleReady[0], ShouldBeTrue)
			So(manifold.taskScaleReady[3], ShouldBeTrue)
		})

		Convey("Shared and skipped caller steps still resolve every issued prediction", func() {
			ledger := NewTemporalLedger(4, head(), NewDirectionalTarget(0.01)).(*TemporalLedger)
			var reading [9]float64

			for _, stepIndex := range []int64{1, 1, 5, 5, 5, 9} {
				reading = observe(ledger, 100+float64(stepIndex)*0.2, stepIndex)
			}

			So(reading[0], ShouldEqual, 2)
		})

		Convey("A zero caller step still resolves through the internal sequence", func() {
			ledger := NewTemporalLedger(4, head(), NewDirectionalTarget(0.01)).(*TemporalLedger)
			var reading [9]float64

			for index := range 6 {
				reading = observe(ledger, 100+float64(index)*0.1, 0)
			}

			So(reading[0], ShouldEqual, 2)
		})
	})
}
