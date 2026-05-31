package perspectives

/*
Branch is a branch in the Perspective's decision tree.
*/
type Branch struct {
	Branches    []Branch
	Category    CategoryType
	Observation ObservationType
	Metric      string
	Regime      Regime
	Condition   ConditionType
	Unit        UnitType
	Value       float64
	ValueSet    bool
	Action      ActionType
}
