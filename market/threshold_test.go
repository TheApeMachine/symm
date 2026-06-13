package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/logic"
)

func TestDerivedEntryBaseline(t *testing.T) {
	testconfig.Load(t)

	Convey("Given a story whose signals produce a low-confidence stream", t, func() {
		baseline := NewBaseline(confidenceBaselineFloor, confidenceBaselineMinObs)

		// Feed a tight cluster of modest confidences — the "typical" reading.
		for i := 0; i < confidenceBaselineMinObs*2; i++ {
			So(baseline.Observe(0.30, confidenceBaselineAlpha), ShouldBeNil)
		}

		tree, treeErr := logic.LoadTree()
		So(treeErr, ShouldBeNil)

		story := &Story{tree: tree, confidenceBaseline: baseline}

		Convey("The derived entry bar warms up and exceeds the typical confidence", func() {
			bar, ok := story.derivedEntryBaseline()
			So(ok, ShouldBeTrue)
			// A signal must STAND OUT against the 0.30 norm, so the bar is well above it.
			So(bar, ShouldBeGreaterThan, 0.30)
		})

		Convey("Before warmup the derived bar is not yet available", func() {
			cold := &Story{tree: tree, confidenceBaseline: NewBaseline(confidenceBaselineFloor, confidenceBaselineMinObs)}
			_, ok := cold.derivedEntryBaseline()
			So(ok, ShouldBeFalse)
		})
	})

	Convey("Given a spread distribution of confidences", t, func() {
		tree, treeErr := logic.LoadTree()
		So(treeErr, ShouldBeNil)

		spread := NewBaseline(confidenceBaselineFloor, confidenceBaselineMinObs)
		for i := 0; i < confidenceBaselineMinObs*4; i++ {
			value := 0.40
			if i%2 == 0 {
				value = 0.60
			}
			So(spread.Observe(value, confidenceBaselineAlpha), ShouldBeNil)
		}

		story := &Story{tree: tree, confidenceBaseline: spread}

		Convey("The derived bar lands strictly above the ~0.50 mean", func() {
			bar, ok := story.derivedEntryBaseline()
			So(ok, ShouldBeTrue)
			So(bar, ShouldBeGreaterThan, 0.50)
			So(bar, ShouldBeLessThanOrEqualTo, tree.ThresholdConfig().EntryConfidenceCeiling)
		})
	})
}
