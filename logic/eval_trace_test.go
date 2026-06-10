package logic

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEvalTraceBottleneck(t *testing.T) {
	Convey("Given a trace that stops at a nested gate", t, func() {
		trace := &EvalTrace{
			Nodes: []TraceNode{
				{Key: "5", Label: "not_holding", Held: true},
				{Key: "5/0", Label: "pumpdump", Held: true},
				{Key: "5/0/0", Label: "hawkes", Held: false, Conditions: []TraceCondition{
					{Label: "hawkes.frenzy", Held: false},
				}},
			},
		}

		Convey("It should return the deepest gate that did not hold", func() {
			bottleneck := trace.Bottleneck()

			So(bottleneck, ShouldNotBeNil)
			So(bottleneck.Key, ShouldEqual, "5/0/0")
			So(trace.FailedConditionLabels(), ShouldResemble, []string{"hawkes.frenzy"})
			So(trace.Depth(), ShouldEqual, 3)
			So(trace.PathKey(), ShouldEqual, "5/0/0")
		})
	})
}

func TestEvalTraceAuditScore(t *testing.T) {
	Convey("Given entry-path traces with different depths", t, func() {
		ignition := &EvalTrace{
			Nodes: []TraceNode{
				{Key: "5", Held: true},
				{Key: "5/0", Held: true},
				{Key: "5/0/0", Held: false},
			},
		}
		scarcity := &EvalTrace{
			Nodes: []TraceNode{
				{Key: "10", Held: false},
			},
		}

		Convey("It should rank the deeper ignition bottleneck higher", func() {
			ignitionDepth, ignitionBranch := ignition.AuditScore()
			scarcityDepth, scarcityBranch := scarcity.AuditScore()

			So(ignitionDepth, ShouldEqual, 3)
			So(ignitionBranch, ShouldEqual, 5)
			So(scarcityDepth, ShouldEqual, 0)
			So(scarcityBranch, ShouldEqual, 999)
			So(ignition.BeatsAuditScore(scarcityDepth, scarcityBranch), ShouldBeTrue)
		})
	})
}

func TestSnapshotSignals(t *testing.T) {
	Convey("Given a measurement window with duplicate sources", t, func() {
		measurements := []Measurement{
			*NewMeasurement(
				SourcePumpDump,
				"BTC/USD",
				0,
				0,
				0,
				0,
				0,
				CategoryVerticalIgnition,
				RegimeTypeNone,
				PositionTypeNone,
				0.72,
				1.4,
			),
			*NewMeasurement(
				SourcePumpDump,
				"BTC/USD",
				0,
				0,
				0,
				0,
				0,
				CategoryCoiledCompression,
				RegimeTypeNone,
				PositionTypeNone,
				0.61,
				0.9,
			),
		}

		Convey("It should keep the latest category per source", func() {
			snapshot := SnapshotSignals(measurements)

			So(snapshot["pumpdump"], ShouldEqual, "coiled_compression@0.61/0.90")
		})
	})
}
