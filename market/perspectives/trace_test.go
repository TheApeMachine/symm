package perspectives

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
)

func TestAcquireTraceRecordStep(t *testing.T) {
	convey.Convey("Given a pooled decision trace", t, func() {
		trace := AcquireTrace(PlaybookTrend)

		convey.Convey("It should record branch steps and reset on release", func() {
			trace.RecordStep(CategoryOrganicTrend, ActionEnter, 1.5, 1.0, ConditionIsGreaterThan, 0, true)
			trace.FinalAction = ActionEnter

			steps := trace.StepsSlice()
			convey.So(len(steps), convey.ShouldEqual, 1)
			convey.So(steps[0].Category, convey.ShouldEqual, CategoryOrganicTrend)
			convey.So(steps[0].SNR, convey.ShouldEqual, 1.5)

			ReleaseTrace(trace)
			reacquired := AcquireTrace(PlaybookDrive)

			convey.So(reacquired.stepCount, convey.ShouldEqual, 0)
			convey.So(reacquired.Playbook, convey.ShouldEqual, PlaybookDrive)

			ReleaseTrace(reacquired)
		})
	})
}

func TestWalkWithTraceRecordsLeaf(t *testing.T) {
	convey.Convey("Given a traversable trend entry tree", t, func() {
		perspective := NewTrendPerspective()
		trace := AcquireTrace(PlaybookTrend)

		action := perspective.DecideWithTrace(trendEntryMeasurements(), nil, trace)

		convey.Convey("It should record the winning branch in the trace", func() {
			convey.So(action, convey.ShouldNotBeNil)
			convey.So(*action, convey.ShouldEqual, ActionEnter)
			convey.So(len(trace.StepsSlice()), convey.ShouldBeGreaterThan, 0)

			ReleaseTrace(trace)
		})
	})
}

func BenchmarkAcquireTraceRelease(b *testing.B) {
	measurements := trendEntryMeasurements()
	perspective := NewTrendPerspective()

	b.ReportAllocs()

	for b.Loop() {
		trace := AcquireTrace(PlaybookTrend)
		_ = perspective.DecideWithTrace(measurements, nil, trace)
		ReleaseTrace(trace)
	}
}
