package perspectives

import (
	"context"
	"embed"
	"fmt"
	"io"

	"go.yaml.in/yaml/v3"
)

//go:embed cfg/perspectives.yaml
var embedded embed.FS

type treeDocument struct {
	Branches BranchList `yaml:"branches"`
}

type Tree struct {
	ctx           context.Context
	cancel        context.CancelFunc
	err           error
	branches      BranchList
	Measurements  []Measurement
	currentAction ActionType
	walkAudit     *WalkAudit
}

func NewTree(
	ctx context.Context, measurements []Measurement,
) (*Tree, error) {
	ctx, cancel := context.WithCancel(ctx)

	cfgReader, err := embedded.Open("cfg/perspectives.yaml")

	if err != nil {
		cancel()
		return nil, fmt.Errorf("perspectives tree: open cfg: %w", err)
	}

	defer cfgReader.Close()

	raw, err := io.ReadAll(cfgReader)

	if err != nil {
		cancel()
		return nil, fmt.Errorf("perspectives tree: read cfg: %w", err)
	}

	document := treeDocument{}

	if err := yaml.Unmarshal(raw, &document); err != nil {
		cancel()
		return nil, fmt.Errorf("perspectives tree: parse cfg: %w", err)
	}

	if document.Branches == nil {
		document.Branches = BranchList{}
	}

	tree := &Tree{
		ctx:          ctx,
		cancel:       cancel,
		branches:     document.Branches.Clone(),
		Measurements: measurements,
	}

	return tree, nil
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

	return tree, nil
}

func (tree *Tree) Action() ActionType {
	return tree.currentAction
}

func (tree *Tree) Err() error {
	return tree.err
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
	tree.currentAction = ActionNone
	tree.walkAudit = nil
}

/*
WalkAudit returns the branch trace from the latest WalkContext call.
*/
func (tree *Tree) WalkAudit() *WalkAudit {
	return tree.walkAudit
}

/*
Walk traverses the tree and returns the Action at the deepest reachable branch.
*/
func (tree *Tree) Walk(measurements []Measurement, branches ...Branch) ActionType {
	return tree.WalkContext(BranchContext{Measurements: measurements}, branches...)
}

/*
WalkContext traverses the tree using the complete branch evaluation context.
*/
func (tree *Tree) WalkContext(
	branchContext BranchContext, branches ...Branch,
) ActionType {
	walkBranches := BranchList(branches)

	if len(walkBranches) == 0 {
		walkBranches = CanonicalPlaybookBranches(tree.branches)
	}

	audit := &WalkAudit{
		Context: branchContext,
	}

	evaluator := NewBranchEvaluator(branchContext)
	tree.currentAction = evaluator.ActionAudited(walkBranches, audit)
	tree.walkAudit = audit
	tree.err = evaluator.Err()

	return tree.currentAction
}
