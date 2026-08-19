package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	logicgraph "github.com/theapemachine/symm/logic/graph"
	"github.com/theapemachine/symm/types"
)

func TestPortfolioRewardOpportunityLifecycle(t *testing.T) {
	Convey("Given a mature accelerating Sudden Pump", t, func() {
		leg := portfolioLeg{
			Symbol: "PUMP/USD",
			Summary: portfolioSummary(0.8),
			Trust:   0.9,
			Opportunity: logicgraph.OpportunityScore{
				Type:      types.OpportunitySuddenPump,
				Lifecycle: types.LifecycleAccelerating,
				Score:     0.9,
			},
		}
		state := NewPortfolioState([]portfolioLeg{leg}, 1, 0)
		postState := state.ApplyAction(portfolioEnterReference(0))

		reward := postState.GetReward()

		Convey("It should earn at full lifecycle weight", func() {
			So(reward, ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given the same pump still only emergent / not yet confirming", t, func() {
		leg := portfolioLeg{
			Symbol: "PUMP/USD",
			Summary: portfolioSummary(0.8),
			Trust:   0.9,
			Opportunity: logicgraph.OpportunityScore{
				Type:      types.OpportunitySuddenPump,
				Lifecycle: types.LifecycleEmergent,
				Score:     0.2,
			},
		}
		state := NewPortfolioState([]portfolioLeg{leg}, 1, 0)
		postState := state.ApplyAction(portfolioEnterReference(0))

		reward := postState.GetReward()

		Convey("It should earn nothing until the evidence confirms", func() {
			So(reward, ShouldEqual, 0)
		})
	})

	Convey("Given a stale, immature observation", t, func() {
		leg := portfolioLeg{
			Symbol: "PUMP/USD",
			Summary: portfolioSummary(0.8),
			Trust:   0.05,
			Opportunity: logicgraph.OpportunityScore{
				Type:      types.OpportunitySuddenPump,
				Lifecycle: types.LifecycleAccelerating,
				Score:     0.8,
			},
		}
		state := NewPortfolioState([]portfolioLeg{leg}, 1, 0)
		postState := state.ApplyAction(portfolioEnterReference(0))

		reward := postState.GetReward()

		Convey("It should be heavily attenuated by epistemic trust", func() {
			So(reward, ShouldBeLessThan, 0.05)
		})
	})
}
