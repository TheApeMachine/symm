package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

func TestPlannerClassifyAllocation(t *testing.T) {
	Convey("Given a return-supported entry whose cognition has not led the basin", t, func() {
		planner := &Planner{}
		thesis := plannerThesisFixture(t, "BTC/USD", 0.90)
		decision := types.NewDecision(types.ActionEnter, "BTC/USD")
		reading, _ := thesis.Resonance.Load("BTC/USD")
		decision.Forecast = reading.(types.ResonanceReading).Forecast

		Convey("It should remain in normal capacity", func() {
			So(planner.classifyAllocation(thesis, decision), ShouldBeNil)
			So(decision.OpportunityMargin, ShouldBeGreaterThan, 0)
			So(decision.CognitiveLead, ShouldEqual, 0)
			So(decision.AllocationClass, ShouldEqual, allocationClassNormal)
			So(decision.Opportunity, ShouldBeFalse)
		})
	})

	Convey("Given return upside above forecast drawdown while cognition leads the basin", t, func() {
		planner := &Planner{}
		thesis := plannerThesisFixture(t, "BTC/USD", 0.90)
		thesis.Manifold.CoherenceMag2 = 0.20
		thesis.Cognition.Store("BTC/USD", types.Cognition{
			Symbol:     "BTC/USD",
			Ready:      true,
			Confidence: 0.80,
		})
		decision := types.NewDecision(types.ActionEnter, "BTC/USD")
		reading, _ := thesis.Resonance.Load("BTC/USD")
		decision.Forecast = reading.(types.ResonanceReading).Forecast

		Convey("It should qualify for reserved overflow capacity", func() {
			So(planner.classifyAllocation(thesis, decision), ShouldBeNil)
			So(decision.Uncertainty, ShouldAlmostEqual, 0.009950166250831947, 1e-12)
			So(decision.OpportunityMargin, ShouldBeGreaterThan, 0)
			So(decision.CognitiveLead, ShouldAlmostEqual, 0.60)
			So(decision.BasinConfidence, ShouldAlmostEqual, 0.80)
			So(decision.AllocationClass, ShouldEqual, allocationClassReserved)
			So(decision.Opportunity, ShouldBeTrue)
		})
	})

	Convey("Given a forecast whose downside exceeds its expected return", t, func() {
		planner := &Planner{}
		forecast, err := types.NewResonanceForecast(
			[]float64{-0.03, 0.04},
			[]float64{1, 1},
			2,
			0.90,
		)
		So(err, ShouldBeNil)
		thesis := types.NewThesis(nil)
		thesis.Cognition.Store("BTC/USD", types.Cognition{
			Symbol:     "BTC/USD",
			Ready:      true,
			Confidence: 0.80,
		})
		decision := types.NewDecision(types.ActionEnter, "BTC/USD")
		decision.Forecast = forecast

		Convey("It should not consume reserve capacity on cognition alone", func() {
			So(planner.classifyAllocation(thesis, decision), ShouldBeNil)
			So(decision.OpportunityMargin, ShouldBeLessThan, 0)
			So(decision.CognitiveLead, ShouldBeGreaterThan, 0)
			So(decision.AllocationClass, ShouldEqual, allocationClassNormal)
			So(decision.Opportunity, ShouldBeFalse)
		})
	})

	Convey("Given an entry without a ready cognition reading", t, func() {
		planner := &Planner{}
		thesis := types.NewThesis(nil)
		decision := types.NewDecision(types.ActionEnter, "BTC/USD")
		decision.Forecast = forecastFixture(t, 0.90)

		Convey("It should fail instead of silently treating missing evidence as normal", func() {
			err := planner.classifyAllocation(thesis, decision)

			So(err, ShouldNotBeNil)
			So(errnie.IsValidation(err), ShouldBeTrue)
		})
	})
}

func BenchmarkPlannerClassifyAllocation(b *testing.B) {
	planner := &Planner{}
	forecast, err := types.NewResonanceForecast(
		[]float64{-0.01, 0.03},
		[]float64{1, 1},
		2,
		0.90,
	)

	if err != nil {
		b.Fatal(err)
	}

	thesis := types.NewThesis(nil)
	thesis.Cognition.Store("BTC/USD", types.Cognition{
		Symbol:     "BTC/USD",
		Ready:      true,
		Confidence: 0.80,
	})
	decision := types.NewDecision(types.ActionEnter, "BTC/USD")
	decision.Forecast = forecast

	for b.Loop() {
		if err := planner.classifyAllocation(thesis, decision); err != nil {
			b.Fatal(err)
		}
	}
}
