package optimizer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
	"go.yaml.in/yaml/v3"
)

/*
WriteBranches writes the optimizer tree document atomically.
*/
func WriteBranches(path string, branches perspectives.BranchList) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("optimizer: empty perspectives output path")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	document := branchDocument{
		Version: 1,
		Branches: branchDocumentsFromBranches(
			perspectives.CanonicalPlaybookBranches(branches),
		),
	}

	raw, err := yaml.Marshal(document)

	if err != nil {
		return err
	}

	temp, err := os.CreateTemp(filepath.Dir(path), ".perspectives-*.yaml")

	if err != nil {
		return err
	}

	tempPath := temp.Name()

	if _, err := temp.Write(raw); err != nil {
		temp.Close()
		os.Remove(tempPath)

		return err
	}

	if err := temp.Close(); err != nil {
		os.Remove(tempPath)

		return err
	}

	return os.Rename(tempPath, path)
}

type branchDocument struct {
	Version  int          `yaml:"version"`
	Branches []branchYAML `yaml:"branches"`
}

type branchYAML struct {
	Branches    []branchYAML                 `yaml:"branches,omitempty" json:"branches,omitempty"`
	Category    perspectives.CategoryType    `yaml:"category,omitempty" json:"category,omitempty"`
	Observation perspectives.ObservationType `yaml:"observation,omitempty" json:"observation,omitempty"`
	Metric      string                       `yaml:"metric,omitempty" json:"metric,omitempty"`
	Regime      perspectives.Regime          `yaml:"regime,omitempty" json:"regime,omitempty"`
	Condition   perspectives.ConditionType   `yaml:"condition,omitempty" json:"condition,omitempty"`
	Unit        perspectives.UnitType        `yaml:"unit,omitempty" json:"unit,omitempty"`
	Value       *float64                     `yaml:"value,omitempty" json:"value,omitempty"`
	ValueSet    bool                         `yaml:"value_set,omitempty" json:"value_set,omitempty"`
	Action      *actionYAML                  `yaml:"action,omitempty" json:"action,omitempty"`
}

type actionYAML struct {
	Type     perspectives.ActionType `yaml:"type" json:"type"`
	Side     trading.Side            `yaml:"side,omitempty" json:"side,omitempty"`
	Symbol   string                  `yaml:"symbol,omitempty" json:"symbol,omitempty"`
	Price    float64                 `yaml:"price,omitempty" json:"price,omitempty"`
	Quantity float64                 `yaml:"quantity,omitempty" json:"quantity,omitempty"`
}

func branchDocumentsFromBranches(
	branches perspectives.BranchList,
) []branchYAML {
	documents := make([]branchYAML, len(branches))

	for index, branch := range branches {
		documents[index] = branchDocumentFromBranch(branch)
	}

	return documents
}

func branchDocumentFromBranch(branch perspectives.Branch) branchYAML {
	document := branchYAML{
		Category:    branch.Category,
		Observation: branch.Observation,
		Metric:      branch.Metric,
		Regime:      branch.Regime,
		Condition:   branch.Condition,
		Unit:        branch.Unit,
		ValueSet:    branch.ValueSet,
	}

	if branch.ValueSet {
		value := branch.Value
		document.Value = &value
	}

	if hasAction(branch.Action) {
		document.Action = actionDocumentFromAction(branch.Action)
	}

	if len(branch.Branches) > 0 {
		document.Branches = branchDocumentsFromBranches(
			perspectives.BranchList(branch.Branches),
		)
	}

	return document
}

func actionDocumentFromAction(action perspectives.Action) *actionYAML {
	return &actionYAML{
		Type:     action.Type,
		Side:     action.Side,
		Symbol:   action.Symbol,
		Price:    action.Price,
		Quantity: action.Quantity,
	}
}

func hasAction(action perspectives.Action) bool {
	return action.Type != perspectives.ActionNone ||
		action.Side != "" ||
		action.Symbol != "" ||
		action.Price != 0 ||
		action.Quantity != 0
}
