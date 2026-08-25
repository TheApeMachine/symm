package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTemporalLedgerNestedHorizonResolution(t *testing.T) {
	Convey("Given a temporal ledger over nested cumulative directional targets", t, func() {
		newLedger := func() *TemporalLedger {
			return NewTemporalLedger(4, DirectionalTarget(0.01))
		}

		head := func() *ResonanceManifold {
			return NewResonanceManifoldWithHorizon([]int{2, 4, 2}, 1, 4, 0.05)
		}

		observe := func(
			ledger *TemporalLedger,
			reference float64,
			manifold *ResonanceManifold,
			callerStep int64,
		) {
			_, err := ledger.Resolve(manifold, callerStep, reference)
			So(err, ShouldBeNil)

			// Predict flat so a resolved ±1 target always carries non-zero
			// error and every horizon row gains scale evidence.
			ledger.Issue(callerStep, reference, make([]float64, 12), []float64{0, 0, 0, 0}, 1)
		}

		Convey("Every horizon of every row resolves against its own cumulative target", func() {
			ledger := newLedger()
			manifold := head()

			for stepIndex := int64(1); stepIndex <= 10; stepIndex++ {
				observe(ledger, 100+float64(stepIndex)*0.1, manifold, stepIndex)
			}

			// A row is fully supervised once four subsequent references have
			// arrived; the final resolve happens before the last issue, so ten
			// steps fully resolve five rows.
			So(ledger.ResolvedCount(), ShouldEqual, 5)

			// Horizon one and horizon four of the supervised rows must both
			// have resolved samples, so the per-horizon head is fully warm.
			_, readyOne := manifold.TaskPrecisionAt(1)
			_, readyFour := manifold.TaskPrecisionAt(4)
			So(readyOne, ShouldBeTrue)
			So(readyFour, ShouldBeTrue)
		})

		Convey("Shared and skipped caller steps still resolve every issued prediction", func() {
			ledger := newLedger()
			manifold := head()

			for _, stepIndex := range []int64{1, 1, 5, 5, 5, 9} {
				observe(ledger, 100+float64(stepIndex)*0.2, manifold, stepIndex)
			}

			So(ledger.ResolvedCount(), ShouldEqual, 1)
		})

		Convey("A zero caller step still resolves through the internal sequence", func() {
			ledger := newLedger()
			manifold := head()

			for index := range 6 {
				observe(ledger, 100+float64(index)*0.1, manifold, 0)
			}

			So(ledger.ResolvedCount(), ShouldEqual, 1)
		})
	})
}
