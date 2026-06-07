package reasoning

import "github.com/theapemachine/symm/market/perspectives/types"

/*
CanonicalPlaybook returns the version-controlled production playbook contract tests expect.

Entry semantics: flat AND (pump evidence via rose_by OR ignition/compression signals)
AND (recent dip via fell_by). The pump and dip legs use different lookbacks so both
can hold on the dip bar without requiring vertical ignition to fire while price falls.
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
						Subject: SubjectPrice,
						Unit:    UnitPercentage,
						Ago:     30,
						Op:      ComparisonRoseBy,
						Value:   8,
					},
					{
						Subject:  SubjectSignal,
						Category: types.CategoryVerticalIgnition,
						Unit:     UnitSNR,
						Op:       ComparisonAtLeast,
						Value:    1,
					},
					{
						Subject:  SubjectSignal,
						Category: types.CategoryCoiledCompression,
						Unit:     UnitSNR,
						Op:       ComparisonAtLeast,
						Value:    1,
					},
				}},
				{
					Subject: SubjectPrice,
					Unit:    UnitPercentage,
					Ago:     8,
					Op:      ComparisonFellBy,
					Value:   3,
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
