package perspectives

import (
	"context"
	"embed"
	"errors"
	"io/fs"
	"slices"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

//go:embed cfg/perspectives.yaml
var embedded embed.FS

type Tree struct {
	ctx           context.Context
	cancel        context.CancelFunc
	err           error
	branches      BranchList
	Measurements  []Measurement
	currentAction *ActionType
}

func NewTree(
	ctx context.Context, measurements []Measurement,
) (*Tree, error) {
	ctx, cancel := context.WithCancel(ctx)

	cfgReader := errnie.Does(func() (fs.File, error) {
		return embedded.Open("cfg/perspectives.yaml")
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	defer cfgReader.Close()

	v := viper.New()
	errnie.Error(v.ReadConfig(cfgReader))

	tree := &Tree{
		ctx:          ctx,
		cancel:       cancel,
		branches:     v.Get("branches").(BranchList),
		Measurements: measurements,
	}

	return tree, errnie.Error(errnie.Require((map[string]any{
		"ctx":          tree.ctx,
		"cancel":       tree.cancel,
		"branches":     tree.branches,
		"measurements": tree.Measurements,
	})))
}

/*
NewTreeFromBranches builds a tree from an in-memory branch registry.
*/
func NewTreeFromBranches(
	ctx context.Context, branches BranchList,
) (*Tree, error) {
	ctx, cancel := context.WithCancel(ctx)

	tree := &Tree{
		ctx:          ctx,
		cancel:       cancel,
		branches:     branches.Clone(),
		Measurements: make([]Measurement, 0),
	}

	return tree, errnie.Error(errnie.Require(map[string]any{
		"ctx":      tree.ctx,
		"cancel":   tree.cancel,
		"branches": tree.branches,
	}))
}

func (tree *Tree) Action() *ActionType {
	return tree.currentAction
}

func (tree *Tree) Branches() BranchList {
	return tree.branches
}

func (tree *Tree) SetBranches(branches BranchList) {
	tree.branches = branches.Clone()
}

func (tree *Tree) AddMeasurement(measurement Measurement) {
	tree.Measurements = append(tree.Measurements, measurement)
}

func (tree *Tree) ResetWalk() {
	tree.currentAction = nil
}

/*
Walk traverses the tree and returns the Action at the deepest reachable leaf the
measurements and observations support. It does not stop at the first branch that
yields an action: every branch is explored as far as the data allows, and the most
specific verdict — the one gated behind the most confirmations — wins. Depth is the
proxy for specificity because each extra level is another category or observation
the measurements had to satisfy to get there. Ties in depth resolve to the earlier
branch, so branch order still expresses priority among equally specific paths.
Branch thresholds on UnitSNR compare against Measurement.SNR (temporal surprise);
UnitConfidence thresholds compare against Measurement.Confidence (instantaneous
clarity). Both are supplied by the signal.
*/
func (tree *Tree) Walk(measurements []Measurement, branches ...Branch) *ActionType {
	for _, branch := range branches {
		index := slices.IndexFunc(measurements, func(measurement Measurement) bool {
			return measurement.Category == branch.Category
		})

		if index >= 0 {
			measurement := measurements[index]

			if tree.passes(measurement, branch) && branch.Action.Type != ActionNone {
				tree.currentAction = &branch.Action.Type
			}
		}

		if len(branch.Branches) > 0 {
			tree.Walk(measurements, branch.Branches...)
		}
	}

	return tree.currentAction
}

func (tree *Tree) passes(measurement Measurement, branch Branch) bool {
	if branch.Condition == ConditionNone || branch.Unit == UnitNone {
		return true
	}

	switch branch.Unit {
	case UnitSNR:
		return tree.compare(measurement.SNR, branch.Value, branch.Condition)
	case UnitConfidence:
		return tree.compare(measurement.Confidence, branch.Value, branch.Condition)
	default:
		errnie.Error(errors.New("unknown unit"), branch.Unit)
	}

	return false
}

func (tree *Tree) compare(
	left, right float64, condition ConditionType,
) bool {
	switch condition {
	case ConditionIsGreaterThan:
		return left > right
	case ConditionIsLessThan:
		return left < right
	case ConditionIsEqual:
		return left == right
	case ConditionIsNotEqual:
		return left != right
	case ConditionIsGreaterThanOrEqual:
		return left >= right
	case ConditionIsLessThanOrEqual:
		return left <= right
	default:
		errnie.Error(errors.New("unknown condition"), condition)
	}

	return false
}
