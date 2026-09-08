package strategy

import (
 "testing"
 "time"

 . "github.com/smartystreets/goconvey/convey"
 "github.com/theapemachine/symm/hindsight"
 "github.com/theapemachine/symm/nomagique/learning/associative/agent"
 "github.com/theapemachine/symm/tests/venue"
)

func TestEvaluationResolve(t *testing.T) {
 Convey("Completed tape legs grade each feasible action in account-return units", t, func() {
  for _, scenario := range []struct {name, kind, end string; reduce bool; expected float64}{
   {"entry captures a rise", "enter", "110", false, .045},
   {"entry loses fees on flat tape", "enter", "100", false, -.005},
   {"late entry loses money", "enter", "90", false, -.055},
   {"exit avoids a decline", "exit", "90", true, .045},
   {"early exit misses a rise", "exit", "110", true, -.055},
   {"holding captures a rise", "hold", "110", false, .05},
   {"holding suffers a decline", "hold", "90", false, -.05},
   {"waiting misses a rise", "wait", "110", false, -.05},
   {"waiting avoids a decline", "wait", "90", false, 0},
   {"increasing captures a rise", "scale", "110", false, .045},
   {"reducing avoids a decline", "scale", "90", true, .045},
  } {
   Convey(scenario.name, func() {
    at := time.Unix(100, 0)
    evaluation := &Evaluation{Decision: &agent.Decision[Action]{ID: 1, Label: "BTC/USD", At: at, Action: Action{Kind: scenario.kind, Reduce: scenario.reduce}}, Initial: venue.Decimal("200"), Quantity: venue.Decimal("1"), Reference: venue.Decimal("100"), Cost: venue.Decimal("100"), Fee: venue.Decimal("1"), Opportunity: venue.Decimal("1")}
    leg := hindsight.Leg{Symbol: "BTC/USD", Through: at, End: venue.Decimal(scenario.end)}
    So(evaluation.Resolve(leg), ShouldBeFalse)
    leg.Through = at.Add(time.Second)
    So(evaluation.Resolve(leg), ShouldBeTrue)
    So(evaluation.Value, ShouldAlmostEqual, scenario.expected)
    So(evaluation.Resolve(leg), ShouldBeFalse)
   })
  }
 })
}
