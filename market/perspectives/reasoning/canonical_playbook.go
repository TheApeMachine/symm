package reasoning

import "github.com/theapemachine/symm/market/perspectives/types"

/*
CanonicalPlaybook returns the version-controlled production playbook contract tests expect.
*/
func CanonicalPlaybook() []Thought {
	return []Thought{
		{
			When: Predicate{All: []Predicate{
				{
					Subject:   SubjectPosition,
					Op:        ComparisonEquals,
					Lifecycle: types.ObservationNotHolding,
				},
				{Any: []Predicate{
					{
						Subject:  SubjectSignal,
						Category: types.CategoryVerticalIgnition,
						Unit:     UnitSNR,
						Ago:      1,
						Op:       ComparisonCrossedUp,
						Value:    1.5,
					},
					{
						Subject:  SubjectSignal,
						Category: types.CategoryOrganicTrend,
						Unit:     UnitSNR,
						Op:       ComparisonAtLeast,
						Value:    1,
					},
				}},
				{
					Subject: SubjectPrice,
					Unit:    UnitPercentage,
					Ago:     20,
					Op:      ComparisonFellBy,
					Value:   2,
				},
			}},
			Do: Act{Type: ActionMarket, Fraction: 0.25},
		},
		{
			When: Predicate{All: []Predicate{
				{
					Subject:   SubjectPosition,
					Op:        ComparisonEquals,
					Lifecycle: types.ObservationHolding,
				},
				{Any: []Predicate{
					{
						Subject:  SubjectSignal,
						Category: types.CategoryActiveReversal,
						Unit:     UnitSNR,
						Op:       ComparisonAtLeast,
						Value:    1,
					},
					{
						Subject:  SubjectSignal,
						Category: types.CategoryFadedExhaustion,
						Unit:     UnitSNR,
						Op:       ComparisonAtLeast,
						Value:    1,
					},
				}},
			}},
			Do: Act{Type: ActionSettlePosition},
		},
		{
			When: Predicate{All: []Predicate{
				{
					Subject:   SubjectPosition,
					Op:        ComparisonEquals,
					Lifecycle: types.ObservationHolding,
				},
				{
					Subject:   SubjectPosition,
					Op:        ComparisonEquals,
					Lifecycle: types.ObservationHasStarted,
				},
			}},
			Do: Act{Type: ActionStopLoss, Offset: 0.012},
		},
		{
			When: Predicate{
				Subject:   SubjectPosition,
				Op:        ComparisonEquals,
				Lifecycle: types.ObservationHolding,
			},
			Do: Act{Type: ActionTrailingStop, Offset: 0.01},
		},
	}
}
