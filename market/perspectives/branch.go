package perspectives

/*
Branch is a branch in the Perspective's decision tree.
*/
type Branch struct {
	Branches          []Branch
	Category          CategoryType
	Observation       ObservationType
	Regime            Regime
	Condition         ConditionType
	Unit              UnitType
	Value             float64
	Action            ActionType
}
