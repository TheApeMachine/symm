package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWalkTraceEvaluationSummary(t *testing.T) {
	Convey("Given a walk trace with exit and entry rejections", t, func() {
		trace := &WalkTrace{
			Symbol: "BTC/USD",
			Steps: []WalkStep{
				{Outcome: StepRejected, Reason: "requires held position, symbol is flat"},
				{Outcome: StepMatched},
				{Outcome: StepRejected, Reason: "pumpdump · confidence ≥ 0.55: 0.2500 ≥ 0.5500 not satisfied"},
			},
		}

		Convey("It should summarize entry blockers and skip exit-only rejections", func() {
			summary := trace.EvaluationSummary()

			So(summary["symbol"], ShouldEqual, "BTC/USD")
			So(summary["matched_steps"], ShouldEqual, 1)
			So(summary["rejected_steps"], ShouldEqual, 2)
			So(summary["entry_blocker"], ShouldEqual, "pumpdump · confidence ≥ 0.55: 0.2500 ≥ 0.5500 not satisfied")
		})
	})
}
