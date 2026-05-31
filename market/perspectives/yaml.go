package perspectives

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

/*
Document is the persisted playbook set loaded from config/perspectives.yaml.
*/
type Document struct {
	Version   int            `yaml:"version"`
	Playbooks []PlaybookSpec `yaml:"playbooks"`
}

type PlaybookSpec struct {
	Name   string       `yaml:"name"`
	Regime string       `yaml:"regime"`
	Policy string       `yaml:"policy"`
	Deny   []BranchSpec `yaml:"deny,omitempty"`
	Entry  []BranchSpec `yaml:"entry"`
	Exit   []BranchSpec `yaml:"exit"`
}

type BranchSpec struct {
	Any         []BranchSpec `yaml:"any,omitempty"`
	All         []BranchSpec `yaml:"all,omitempty"`
	Branches    []BranchSpec `yaml:"branches,omitempty"`
	Category    string       `yaml:"category,omitempty"`
	Observation string       `yaml:"observation,omitempty"`
	Metric      string       `yaml:"metric,omitempty"`
	Unit        string       `yaml:"unit,omitempty"`
	Condition   string       `yaml:"condition,omitempty"`
	Value       *float64     `yaml:"value,omitempty"`
	Action      string       `yaml:"action,omitempty"`
}

func LoadDocumentFile(path string) (Document, error) {
	payload, err := os.ReadFile(path)

	if err != nil {
		return Document{}, err
	}

	return DecodeDocument(payload)
}

func SaveDocumentFile(path string, document Document) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	payload, err := yaml.Marshal(document)

	if err != nil {
		return err
	}

	return os.WriteFile(path, payload, 0o644)
}

func DecodeDocument(payload []byte) (Document, error) {
	var document Document

	if err := yaml.Unmarshal(payload, &document); err != nil {
		return Document{}, err
	}

	if document.Version <= 0 {
		return Document{}, fmt.Errorf("perspectives version is required")
	}

	if len(document.Playbooks) == 0 {
		return Document{}, fmt.Errorf("at least one playbook is required")
	}

	return document, nil
}

func BuildStrategies(document Document) ([]Perspective, error) {
	strategies := make([]Perspective, 0, len(document.Playbooks))

	for _, playbook := range document.Playbooks {
		strategy, err := buildStrategy(playbook)

		if err != nil {
			return nil, fmt.Errorf("playbook %q: %w", playbook.Name, err)
		}

		strategies = append(strategies, strategy)
	}

	return strategies, nil
}

func buildStrategy(spec PlaybookSpec) (*strategy, error) {
	name, err := parsePlaybookName(spec.Name)

	if err != nil {
		return nil, err
	}

	regime, err := parseRegime(spec.Regime)

	if err != nil {
		return nil, err
	}

	policy, err := parsePolicy(spec.Policy)

	if err != nil {
		return nil, err
	}

	entry, err := buildBranches(spec.Entry)

	if err != nil {
		return nil, fmt.Errorf("entry: %w", err)
	}

	exit, err := buildExit(spec.Exit)

	if err != nil {
		return nil, fmt.Errorf("exit: %w", err)
	}

	strategy := newStrategy(name, regime, policy, entry, exit)

	if spec.Deny != nil {
		deny, denyErr := buildBranches(spec.Deny)

		if denyErr != nil {
			return nil, fmt.Errorf("deny: %w", denyErr)
		}

		strategy.deny = &Tree{Branches: deny}
	}

	return strategy, nil
}

func buildExit(specs []BranchSpec) (Branch, error) {
	branches, err := buildBranches(specs)

	if err != nil {
		return Branch{}, err
	}

	return Branch{Observation: ObservationHolding, Branches: branches}, nil
}

func buildBranches(specs []BranchSpec) ([]Branch, error) {
	branches := make([]Branch, 0, len(specs))

	for _, spec := range specs {
		built, err := buildBranchSet(spec)

		if err != nil {
			return nil, err
		}

		branches = append(branches, built...)
	}

	return branches, nil
}

func buildBranchSet(spec BranchSpec) ([]Branch, error) {
	if len(spec.Any) > 0 {
		return buildBranches(spec.Any)
	}

	branch, err := buildBranch(spec)

	if err != nil {
		return nil, err
	}

	return []Branch{branch}, nil
}

func buildBranch(spec BranchSpec) (Branch, error) {
	if len(spec.All) > 0 {
		return buildAll(spec.All)
	}

	branch, err := buildPrimitiveBranch(spec)

	if err != nil {
		return Branch{}, err
	}

	if len(spec.Branches) > 0 {
		children, childErr := buildBranches(spec.Branches)

		if childErr != nil {
			return Branch{}, childErr
		}

		branch.Branches = children
	}

	return branch, nil
}

func buildAll(specs []BranchSpec) (Branch, error) {
	if len(specs) == 0 {
		return Branch{}, fmt.Errorf("all block requires children")
	}

	branch, err := buildBranch(specs[0])

	if err != nil {
		return Branch{}, err
	}

	if len(specs) == 1 {
		return branch, nil
	}

	child, childErr := buildAll(specs[1:])

	if childErr != nil {
		return Branch{}, childErr
	}

	branch.Branches = append(branch.Branches, child)

	return branch, nil
}

func cleanName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
