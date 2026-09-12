package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTemporalLedgerNestedHorizonResolution(t *testing.T) {
	Convey("Given a temporal ledger over nested cumulative directional targets", t, func() {
		newLedger := func(manifold *ResonanceManifold) *TemporalLedger {
			return NewTemporalLedger(
				4,
				manifold,
				NewDirectionalTarget(0.01),
			).(*TemporalLedger)
		}

		head := func() *ResonanceManifold {
			return NewResonanceManifold([]int{2, 4, 2}, 1, 4, 0.05, ReadoutAll).(*ResonanceManifold)
		}

		observe := func(
			ledger *TemporalLedger,
			reference float64,
			manifold *ResonanceManifold,
			callerStep int64,
		) {
			err := ledger.resolve(&ResolveIntent{Step: callerStep, Reference: reference})
			So(err, ShouldBeNil)

			// Predict flat so a resolved ±1 target always carries non-zero
			// error and every horizon row gains scale evidence.
			ledger.issue(&IssueIntent{
				Step:        callerStep,
				Reference:   reference,
				Features:    make([]float64, 12),
				Predictions: []float64{0, 0, 0, 0},
				Horizon:     1,
			})
		}

		Convey("Every horizon of every row resolves against its own cumulative target", func() {
			manifold := head()
			ledger := newLedger(manifold)

			for stepIndex := int64(1); stepIndex <= 10; stepIndex++ {
				observe(ledger, 100+float64(stepIndex)*0.1, manifold, stepIndex)
			}

			// A row is fully supervised once four subsequent references have
			// arrived; ten steps fully resolve six rows (1-5, 2-6, 3-7, 4-8, 5-9, 6-10).
			So(ledger.resolved, ShouldEqual, 6)

			// Horizon one and horizon four of the supervised rows must both
			// have resolved samples, so the per-horizon head is fully warm.
			So(manifold.taskScaleReady[0], ShouldBeTrue)
			So(manifold.taskScaleReady[3], ShouldBeTrue)
		})

		Convey("Shared and skipped caller steps still resolve every issued prediction", func() {
			manifold := head()
			ledger := newLedger(manifold)

			for _, stepIndex := range []int64{1, 1, 5, 5, 5, 9} {
				observe(ledger, 100+float64(stepIndex)*0.2, manifold, stepIndex)
			}

			So(ledger.resolved, ShouldEqual, 2)
		})

		Convey("A zero caller step still resolves through the internal sequence", func() {
			manifold := head()
			ledger := newLedger(manifold)

			for index := range 6 {
				observe(ledger, 100+float64(index)*0.1, manifold, 0)
			}

			So(ledger.resolved, ShouldEqual, 2)
		})
	})
}
