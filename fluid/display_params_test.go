package fluid

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestDisplayParamsApplyValidatesRanges(t *testing.T) {
	convey.Convey("Given default display params", t, func() {
		params := NewDisplayParams()
		alpha := 0.01

		convey.Convey("It should reject out-of-range EMA alpha", func() {
			_, err := params.Apply(DisplayPatch{HeightEMAAlpha: &alpha})

			convey.So(err, convey.ShouldNotBeNil)
		})
	})
}

func TestDisplayParamsApplyUpdatesSnapshot(t *testing.T) {
	convey.Convey("Given default display params", t, func() {
		params := NewDisplayParams()
		alpha := 0.55

		convey.Convey("It should apply a valid EMA alpha patch", func() {
			_, err := params.Apply(DisplayPatch{HeightEMAAlpha: &alpha})

			convey.So(err, convey.ShouldBeNil)
			convey.So(params.Snapshot().HeightEMAAlpha, convey.ShouldEqual, 0.55)
		})
	})
}

func TestGridBuilderResetSmoothing(t *testing.T) {
	params := NewDisplayParams()
	builder := NewGridBuilder(params)
	first := builder.Build(sampleGridRows(100), params.activeGridSize())

	shifted := sampleGridRows(100)

	for index := range shifted {
		shifted[index].Re *= 2
	}

	builder.Build(shifted, params.activeGridSize())
	builder.ResetSmoothing()
	afterReset := builder.Build(shifted, params.activeGridSize())

	if first.Heights[0][0] == afterReset.Heights[0][0] {
		t.Fatal("expected reset smoothing to drop prior EMA state")
	}
}
