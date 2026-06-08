package logic

import "github.com/theapemachine/errnie"

type Tree struct {
	branches []*Branch
}

func NewTree() *Tree {
	return &Tree{
		branches: make([]*Branch, 0),
	}
}

func (tree *Tree) Evaluate(measurements []Measurement) *Action {
	for _, branch := range tree.branches {
		action, err := branch.Evaluate(measurements)

		if errnie.Error(err) != nil {
			continue
		}

		if action != nil {
			return action
		}
	}

	return nil
}
