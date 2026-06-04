package perspectives

/*
Branch is a branch in the Perspective's decision tree.
*/
type Branch struct {
	Branches         []Branch
	Category         CategoryType
	Observation      ObservationType
	Metric           string
	Regime           Regime
	Condition        ConditionType
	ConditionBoolean ConditionBooleanType
	Unit             UnitType
	Value            float64
	ValueSet         bool
	Action           Action
}

/*
BranchList is a ordered branch registry for one tree level.
*/
type BranchList []Branch

/*
Clone deep-copies nested branches.
*/
func (list BranchList) Clone() BranchList {
	cloned := make(BranchList, len(list))

	for index, branch := range list {
		cloned[index] = branch.Clone()
	}

	return cloned
}

/*
Clone deep-copies one branch subtree.
*/
func (branch Branch) Clone() Branch {
	clone := branch

	if len(branch.Branches) > 0 {
		clone.Branches = BranchList(branch.Branches).Clone()
	}

	return clone
}
