package logic

import (
	"context"
	"embed"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"go.yaml.in/yaml/v3"
)

//go:embed rules/tree.yml
var rules embed.FS

type Tree struct {
	ctx      context.Context
	cancel   context.CancelFunc
	pool     *qpool.Q[any]
	Branches []*Branch `yaml:"branches" json:"branches"`
}

func NewTree(
	ctx context.Context,
	pool *qpool.Q[any],
) (*Tree, error) {
	ctx, cancel := context.WithCancel(ctx)

	tree := &Tree{
		ctx:      ctx,
		cancel:   cancel,
		pool:     pool,
		Branches: make([]*Branch, 0),
	}

	raw, err := rules.ReadFile("rules/tree.yml")

	if err != nil {
		cancel()
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"logic: read tree rules",
			err,
		))
	}

	if err := yaml.Unmarshal(raw, tree); err != nil {
		cancel()
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"logic: decode tree rules",
			err,
		))
	}

	return tree, nil
}

func (tree *Tree) Evaluate(
	measurements []*datura.Artifact,
	holdings *datura.Artifact,
	branches []*Branch,
) ([]*Action, error) {
	actions := make([]*Action, 0)

	for _, branch := range branches {
		candidates, err := branch.Evaluate(measurements, holdings)
		if err != nil {
			return nil, err
		}

		actions = append(actions, candidates...)
	}

	return actions, nil
}
