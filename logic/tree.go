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

	return tree, nil
}

func (tree *Tree) Evaluate(measurements []Measurement) *Action {
	evalContext := NewEvalContext(measurements)

	for _, branch := range tree.Branches {
		action, err := branch.Evaluate(measurements, evalContext)

		if errnie.Error(err) != nil {
			continue
		}

		if action != nil {
			return action
		}
	}

	return nil
}
