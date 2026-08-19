package learning

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTemporalLedgerResolveUnderRepeatedSteps(t *testing.T) {
	Convey("Given a temporal ledger over a strict-prior directional target", t, func() {
		newLedger := func() *TemporalLedger {
			return NewTemporalLedger(4, DirectionalTarget(0.01))
		}

		manifold := func() *ResonanceManifold {
			return NewResonanceManifold([]int{2, 4, 2}, 1, 0.05)
		}

		observe := func(ledger *TemporalLedger, reference float64, head *ResonanceManifold, callerStep int64) {
			_, err := ledger.Resolve(head, callerStep, reference)
			So(err, ShouldBeNil)

			ledger.Issue(callerStep, reference, make([]float64, 12), 1, 1)
		}

		Convey("Consecutive caller steps resolve one sample per reference", func() {
			ledger := newLedger()
			head := manifold()

			for stepIndex := int64(1); stepIndex <= 10; stepIndex++ {
				observe(ledger, 100+float64(stepIndex)*0.1, head, stepIndex)
			}

			So(ledger.ResolvedCount(), ShouldEqual, 9)
		})

		Convey("Shared and skipped caller steps resolve every issued prediction", func() {
			ledger := newLedger()
			head := manifold()

			for _, stepIndex := range []int64{1, 1, 5, 5, 5, 9} {
				observe(ledger, 100+float64(stepIndex)*0.2, head, stepIndex)
			}

			So(ledger.ResolvedCount(), ShouldEqual, 5)
		})

		Convey("A zero caller step still resolves through the internal sequence", func() {
			ledger := newLedger()
			head := manifold()

			for index := range 6 {
				observe(ledger, 100+float64(index)*0.1, head, 0)
			}

			So(ledger.ResolvedCount(), ShouldEqual, 5)
		})
	})
}
