package perspectives

/*
WalkAudit records one playbook tree traversal with per-branch predicate outcomes.
*/
type WalkAudit struct {
	Symbol       string
	Trigger      Measurement
	Context      BranchContext
	Steps        []BranchStep
	Verdict      *ActionType
	VerdictDepth int
	SelectedPath []BranchPathNode
}

/*
BranchStep is one branch predicate evaluation at one tree depth.
*/
type BranchStep struct {
	Depth      int              `json:"depth"`
	Index      int              `json:"index"`
	Branch     BranchDescriptor `json:"branch"`
	Pass       bool             `json:"pass"`
	FailReason string           `json:"fail_reason,omitempty"`
	Compared   *ComparedValue   `json:"compared,omitempty"`
	OnPath     bool             `json:"on_path,omitempty"`
}

/*
BranchPathNode identifies one node on the winning branch chain.
*/
type BranchPathNode struct {
	Depth int `json:"depth"`
	Index int `json:"index"`
}

/*
BranchDescriptor summarizes the branch predicate under test.
*/
type BranchDescriptor struct {
	Category    CategoryType    `json:"category,omitempty"`
	Observation ObservationType `json:"observation,omitempty"`
	Regime      Regime          `json:"regime,omitempty"`
	Metric      string          `json:"metric,omitempty"`
	Condition   ConditionType   `json:"condition,omitempty"`
	Unit        UnitType        `json:"unit,omitempty"`
	Threshold   float64         `json:"threshold,omitempty"`
	Action      ActionType      `json:"action,omitempty"`
}

/*
ComparedValue captures the numeric or boolean predicate that decided pass/fail.
*/
type ComparedValue struct {
	Field string        `json:"field,omitempty"`
	Left  float64       `json:"left"`
	Op    ConditionType `json:"op,omitempty"`
	Right float64       `json:"right"`
	Pass  bool          `json:"pass"`
}

type branchCheck struct {
	pass     bool
	reason   string
	compared *ComparedValue
}

func branchDescriptor(branch Branch) BranchDescriptor {
	return BranchDescriptor{
		Category:    branch.Category,
		Observation: branch.Observation,
		Regime:      branch.Regime,
		Metric:      branch.Metric,
		Condition:   branch.Condition,
		Unit:        branch.Unit,
		Threshold:   branch.Value,
		Action:      branch.Action.Type,
	}
}

func (audit *WalkAudit) recordStep(step BranchStep) {
	audit.Steps = append(audit.Steps, step)
}

func (audit *WalkAudit) markSelectedPath(path []BranchPathNode) {
	if len(path) == 0 {
		return
	}

	audit.SelectedPath = append([]BranchPathNode(nil), path...)

	for stepIndex := range audit.Steps {
		step := &audit.Steps[stepIndex]

		for _, node := range path {
			if step.Depth == node.Depth && step.Index == node.Index {
				step.OnPath = true
			}
		}
	}
}
