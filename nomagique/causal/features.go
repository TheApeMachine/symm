package causal

import "github.com/theapemachine/symm/nomagique/core"

/*
Query is one observational table, the declared predictors, and the intervention.
The caller declares the exact feature list; no automatic removal, addition,
ordering or deduplication of predictors occurs here.
*/
type Query struct {
	Rows      [][]float64
	Features  []int
	Target    int
	Treatment int
	Level     float64
	Actual    []float64
}

/*
Features reports whether the treatment is represented and the outcome is
excluded from the predictors.
*/
func Features(query Query) bool {
	hasTreatment := false
	hasTarget := false

	for _, index := range query.Features {
		if index == query.Treatment {
			hasTreatment = true
		}

		if index == query.Target {
			hasTarget = true
		}
	}

	return hasTreatment && !hasTarget
}

func shape(query Query) error {
	if !Features(query) {
		return core.ErrDomain
	}

	return nil
}
