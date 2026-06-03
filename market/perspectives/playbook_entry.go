package perspectives

import "fmt"

/*
EntryPassMeasurements builds measurement rows that satisfy the entry path of the
given branch registry (e.g. an optimizer candidate). Tests must use
FixturePlaybookBranches and FixturePlaybookEntryMeasurements instead of the
embedded cfg/perspectives.yaml, which the optimizer rewrites.
*/
func EntryPassMeasurements(
	symbol string, last float64, branches BranchList,
) ([]Measurement, error) {
	if symbol == "" {
		return nil, fmt.Errorf("entry pass measurements: empty symbol")
	}

	if last <= 0 {
		return nil, fmt.Errorf("entry pass measurements: last must be positive")
	}

	canonical := CanonicalPlaybookBranches(branches)
	entryIndex := FindEntryIndex(canonical)

	if entryIndex < 0 {
		return nil, fmt.Errorf("entry pass measurements: no entry branch")
	}

	gates, found := entryPassGates(canonical[entryIndex])

	if !found || len(gates) == 0 {
		return nil, fmt.Errorf("entry pass measurements: no entry path")
	}

	rows := make([]Measurement, len(gates))

	for index, gate := range gates {
		rows[index] = Measurement{
			Symbol:   symbol,
			Category: gate.category,
			SNR:      passMeasurementSNR(gate.branch),
			Last:     last,
		}
	}

	if err := verifyEntryPassMeasurements(rows, canonical); err != nil {
		return nil, err
	}

	return rows, nil
}

type entryGate struct {
	category CategoryType
	branch   Branch
}

func entryPassGates(entryBranch Branch) ([]entryGate, bool) {
	path, found := entryPathToLeaf(entryBranch)

	if !found {
		return nil, false
	}

	gates := make([]entryGate, 0, len(path))

	for _, branch := range path {
		if branch.Category == CategoryTypeNone || isPreEntryDenyCategory(branch.Category) {
			continue
		}

		gates = append(gates, entryGate{category: branch.Category, branch: branch})
	}

	return gates, true
}

func entryPathToLeaf(branch Branch) ([]Branch, bool) {
	if isEntryLeaf(branch) {
		return []Branch{branch}, true
	}

	for _, child := range branch.Branches {
		childPath, found := entryPathToLeaf(child)

		if !found {
			continue
		}

		return append([]Branch{branch}, childPath...), true
	}

	return nil, false
}

func isPreEntryDenyCategory(category CategoryType) bool {
	switch category {
	case CategoryToxicBluff:
		return true
	default:
		return false
	}
}

func isEntryLeaf(branch Branch) bool {
	return branch.Observation == ObservationNotHolding &&
		IsEntryAction(branch.Action.Type)
}

func passMeasurementSNR(branch Branch) float64 {
	if !branch.ValueSet {
		return 1.0
	}

	return branch.Value + 1.0
}

func verifyEntryPassMeasurements(rows []Measurement, branches BranchList) error {
	evaluator := NewBranchEvaluator(BranchContext{
		Measurements: rows,
		Observations: map[ObservationType]float64{
			ObservationNotHolding: 1,
		},
	})
	action := evaluator.Action(branches)

	if evaluator.Err() != nil {
		return fmt.Errorf("entry pass measurements: %w", evaluator.Err())
	}

	if action == nil || !IsEntryAction(*action) {
		return fmt.Errorf("entry pass measurements: walk did not reach entry action")
	}

	return nil
}
