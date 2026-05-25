package trader

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
)

func TestDecisionEngineBuildReportsEdge(t *testing.T) {
	Convey("Given an executable candidate below round-trip cost", t, func() {
		candidates := NewCandidateStore()
		candidates.Note(SignalCandidate{
			Symbol:         "PUMP/EUR",
			Source:         "basis",
			Confidence:     1,
			ExpectedReturn: 0.001,
			Runway:         time.Minute,
			Direction:      1,
			Executable:     true,
		})

		decisionEngine := DecisionEngine{}
		snapshot := decisionEngine.Build(
			candidates,
			stubPrices{"PUMP/EUR": 100},
			stubMarket{snapshots: map[string]engine.Snapshot{
				"PUMP/EUR": {LastOK: true, SpreadOK: true, BatchOK: true},
			}},
			time.Now(),
			200,
			false,
			EnsembleContext{Regime: RegimeTrending},
		)

		row := snapshot.Evaluations[0]
		payload := evaluationToMap(row)

		Convey("It should expose the required edge and deficit", func() {
			So(row.Allow, ShouldBeFalse)
			So(row.Why, ShouldEqual, "negative_edge")
			So(row.RequiredEdge, ShouldBeGreaterThan, row.ExpectedReturn)
			So(row.EdgeSurplus, ShouldBeLessThan, 0)
			So(payload["required_edge"], ShouldAlmostEqual, row.RequiredEdge)
			So(payload["edge_surplus"], ShouldAlmostEqual, row.EdgeSurplus)
		})
	})
}
