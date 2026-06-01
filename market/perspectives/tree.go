package perspectives

import (
	"context"
	"errors"
	"os"
	"slices"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

type Tree struct {
	ctx           context.Context
	cancel        context.CancelFunc
	err           error
	branches      []Branch
	measurements  []Measurement
	currentAction *ActionType
}

func NewTree(
	ctx context.Context, measurements []Measurement,
) (*Tree, error) {
	ctx, cancel := context.WithCancel(ctx)

	fh := errnie.Does(func() (*os.File, error) {
		return os.Open("perspectives.yaml")
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	defer fh.Close()

	v := viper.New()
	v.ReadConfig(fh)

	tree := &Tree{
		ctx:          ctx,
		cancel:       cancel,
		branches:     v.Get("branches").([]Branch),
		measurements: measurements,
	}

	return tree, errnie.Error(errnie.Require((map[string]any{
		"ctx":          tree.ctx,
		"cancel":       tree.cancel,
		"branches":     tree.branches,
		"measurements": tree.measurements,
	})))
}

/*
Walk traverses the tree and returns the Action at the deepest reachable leaf the
measurements and observations support. It does not stop at the first branch that
yields an action: every branch is explored as far as the data allows, and the most
specific verdict — the one gated behind the most confirmations — wins. Depth is the
proxy for specificity because each extra level is another category or observation
the measurements had to satisfy to get there. Ties in depth resolve to the earlier
branch, so branch order still expresses priority among equally specific paths.
Branch thresholds on UnitSNR compare against Measurement.SNR supplied by the signal.
*/
func (tree *Tree) Walk(measurements []Measurement, branches []Branch) *ActionType {
	for _, branch := range branches {
		if slices.ContainsFunc(measurements, func(measurement Measurement) bool {
			return measurement.Category == branch.Category
		}) {
			if branch.Condition != ConditionNone {
				if branch.Unit != UnitNone {
					tree.handleUnit(measurement, branch)
				}
			}

			if branch.Action != ActionNone {
				tree.currentAction = &branch.Action
			}
		}

		if len(branch.Branches) > 0 {
			tree.Walk(measurements, branch.Branches)
		}
	}

	return tree.currentAction
}

func (tree *Tree) handleUnit(
	measurement Measurement, branch Branch,
) {
	switch branch.Unit {
	case UnitSNR:
		tree.handleCondition(
			measurement.SNR, branch.Value, branch.Condition,
		)
	default:
		errnie.Error(errors.New("unknown unit"), branch.Unit)
	}
}

func (tree *Tree) handleCondition(
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
