package logic

import (
	"context"
	"embed"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/user"
	"go.yaml.in/yaml/v3"
)

//go:embed rules/tree.yml
var embedded embed.FS

type Tree struct {
	ctx      context.Context
	cancel   context.CancelFunc
	pool     *qpool.Q[any]
	Branches []*Branch `yaml:"branches" json:"branches"`
}

/*
NewTree decodes the embedded playbook for story evaluation and dashboard replay.
*/
func NewTree(ctx context.Context, pool *qpool.Q[any]) (*Tree, error) {
	reader, err := embedded.Open("rules/tree.yml")

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"logic: failed to open tree rules",
			err,
		))
	}

	defer reader.Close()

	ctx, cancel := context.WithCancel(ctx)
	tree := &Tree{
		ctx:      ctx,
		cancel:   cancel,
		pool:     pool,
		Branches: make([]*Branch, 0),
	}

	if err := yaml.NewDecoder(reader).Decode(tree); err != nil {
		return tree, errnie.Error(errnie.Err(
			errnie.IO,
			"logic: failed to decode tree rules",
			err,
		))
	}

	return tree, nil
}

/*
Evaluate walks the decision tree and returns
a slice of all successful evaluations.
*/
func (tree *Tree) Evaluate(
	measurements []Measurement,
	holdings *user.Balances,
	branches []*Branch,
) (results []*Action, err error) {
	if len(measurements) == 0 {
		return nil, nil
	}

	for _, branch := range branches {
		action, evaluateErr := branch.Evaluate(measurements, holdings)

		if evaluateErr != nil {
			return results, evaluateErr
		}

		if action != nil {
			results = append(results, action)
		}
	}

	return results, nil
}
