package logic

import (
	"embed"

	"github.com/theapemachine/errnie"
	"go.yaml.in/yaml/v3"
)

//go:embed rules/tree.yml
var embedded embed.FS

type Tree struct {
	Branches []*Branch `yaml:"branches"`
	stats    *TreeStats
}

func NewTree() (*Tree, error) {
	reader, err := embedded.Open("rules/tree.yml")

	if err != nil {
		return nil, err
	}

	defer reader.Close()

	tree := &Tree{}

	if err := yaml.NewDecoder(reader).Decode(tree); errnie.Error(err) != nil {
		return tree, err
	}

	tree.stats = NewTreeStats(tree.Branches, 32)

	return tree, nil
}

func (tree *Tree) Evaluate(measurements []Measurement, holdings *Holdings) *Evaluation {
	evalContext := NewEvalContext(measurements, holdings)

	if tree.stats != nil {
		tree.stats.BeginEvaluation()
	}

	for branchIndex, branch := range tree.Branches {
		evaluation, err := branch.Evaluate(
			measurements,
			evalContext,
		)

		if errnie.Error(err) != nil {
			continue
		}

		if evaluation != nil {
			return evaluation
		}

		if branchIndex < firstEntryBranchIndex {
			continue
		}
	}

	return nil
}

/*
Stats exposes playbook instrumentation for dashboard publishing.
*/
func (tree *Tree) Stats() *TreeStats {
	return tree.stats
}
