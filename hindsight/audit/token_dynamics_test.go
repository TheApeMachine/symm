package audit

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestComputeTransitionEntropy(t *testing.T) {
	Convey("Given transition matrices for conditional entropy calculation", t, func() {
		Convey("When transitions are completely empty", func() {
			entropy := computeTransitionEntropy(nil)
			So(entropy, ShouldEqual, 0.0)
		})

		Convey("When transitions are deterministic", func() {
			transitions := map[string]map[string]int{
				"R00": {"R01": 10},
				"R01": {"R02": 20},
				"R02": {"R00": 30},
			}

			entropy := computeTransitionEntropy(transitions)
			So(entropy, ShouldEqual, 0.0)
		})

		Convey("When each state transitions uniformly to two possible next states", func() {
			transitions := map[string]map[string]int{
				"R00": {"R00": 5, "R01": 5},
				"R01": {"R00": 10, "R01": 10},
			}

			// Each row has H(Y|X=x) = - (0.5 log2 0.5 + 0.5 log2 0.5) = 1.0 bit
			// P(X=R00) = 10/30, P(X=R01) = 20/30
			// Sum P(X=x) H(Y|X=x) = (10/30)*1.0 + (20/30)*1.0 = 1.0 bit
			entropy := computeTransitionEntropy(transitions)
			So(entropy, ShouldAlmostEqual, 1.0, 1e-12)
		})

		Convey("When row frequencies are asymmetric across the transition ensemble", func() {
			transitions := map[string]map[string]int{
				"R00": {"R01": 5, "R02": 5}, // 10 transitions, row entropy = 1.0 bit
				"R01": {"R00": 90},           // 90 transitions, row entropy = 0.0 bit
			}

			// Total transitions = 100
			// Weight for R00 = 10 / 100 = 0.1
			// Weight for R01 = 90 / 100 = 0.9
			// Conditional entropy = 0.1 * 1.0 + 0.9 * 0.0 = 0.1 bit
			entropy := computeTransitionEntropy(transitions)
			So(entropy, ShouldAlmostEqual, 0.1, 1e-12)
		})
	})
}
