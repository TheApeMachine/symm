package progress

import (
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/optimizer/types"
)

func TestSeedSearchTargetDepth(t *testing.T) {
	convey.Convey("Given beam survivors with deny-inflated depth and shallow traded seeds", t, func() {
		shallow := types.CandidateScore{
			ClosedTrades: 10,
			Branches: perspectives.BranchList{
				{
					Category: perspectives.CategoryLaminar,
					Branches: []perspectives.Branch{
						{
							Category:    perspectives.CategoryLaminar,
							Observation: perspectives.ObservationNotHolding,
							Action:      perspectives.Action{Type: perspectives.ActionLimit},
						},
					},
				},
			},
		}
		deep := types.CandidateScore{
			ClosedTrades: 0,
			Branches: perspectives.BranchList{
				{
					Category: perspectives.CategoryToxicBluff,
					Branches: []perspectives.Branch{
						{
							Category: perspectives.CategorySaturation,
							Branches: []perspectives.Branch{
								{
									Category: perspectives.CategoryTurbulent,
									Branches: []perspectives.Branch{
										{
											Category: perspectives.CategoryLiquidityShock,
											Branches: []perspectives.Branch{
												{
													Category: perspectives.CategoryMechanicalCollapse,
													Branches: []perspectives.Branch{
														{
															Category: perspectives.CategorySystemicBeta,
															Branches: []perspectives.Branch{
																{
																	Category:    perspectives.CategoryLaminar,
																	Observation: perspectives.ObservationNotHolding,
																	Action:      perspectives.Action{Type: perspectives.ActionLimit},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		}

		convey.Convey("It should start from the shallowest traded depth", func() {
			convey.So(SeedSearchTargetDepth([]types.CandidateScore{deep, shallow}), convey.ShouldEqual, 2)
			convey.So(maxReasoningDepthInBeam([]types.CandidateScore{deep, shallow}), convey.ShouldEqual, 7)
		})
	})
}
