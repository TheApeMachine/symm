package broker

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDeskFlowStats(t *testing.T) {
	Convey("Given accumulated broker flow counters", t, func() {
		desk := &Desk{}
		desk.submittedCount.Add(3)
		desk.filledCount.Add(2)
		desk.preflightRejectedCount.Add(1)

		first := desk.FlowStats()
		second := desk.FlowStats()

		Convey("It should report stats without clearing counters", func() {
			So(first.SubmittedCount, ShouldEqual, 3)
			So(first.FilledCount, ShouldEqual, 2)
			So(first.PreflightRejectedCount, ShouldEqual, 1)
			So(second.SubmittedCount, ShouldEqual, 3)
			So(second.FilledCount, ShouldEqual, 2)
			So(second.PreflightRejectedCount, ShouldEqual, 1)
		})
	})
}

func TestDeskRecordExecutionFlow(t *testing.T) {
	Convey("Given an execution status with exchange casing", t, func() {
		desk := &Desk{}

		desk.recordExecutionFlow(nil, "FILLED")

		Convey("It should count the filled execution", func() {
			So(desk.FlowStats().FilledCount, ShouldEqual, 1)
		})
	})
}
