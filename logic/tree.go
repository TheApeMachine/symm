package logic

import (
	"embed"
	"fmt"

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

	applyConfigThresholds(tree)

	return tree, nil
}

func (tree *Tree) Evaluate(measurements []Measurement, holdings *Holdings) (*Evaluation, error) {
	for index, branch := range tree.Branches {
		evaluation, err := branch.Evaluate(measurements, fmt.Sprintf("%d", index), holdings)

		if errnie.Error(err) != nil {
			return nil, errnie.Error(err)
		}

		if evaluation != nil {
			return evaluation, nil
		}
	}

	return nil, nil
}
