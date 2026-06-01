package perspectives

import (
	"context"
	"embed"
	"io"
	"io/fs"

	"github.com/theapemachine/errnie"
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
	currentAction *ActionType
}

func NewTree(
	ctx context.Context, measurements []Measurement,
) (*Tree, error) {
	ctx, cancel := context.WithCancel(ctx)

	cfgReader := errnie.Does(func() (fs.File, error) {
		return embedded.Open("cfg/perspectives.yaml")
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	defer cfgReader.Close()

	raw, err := io.ReadAll(cfgReader)

	if err != nil {
		cancel()
		return nil, errnie.Error(err)
	}

	document := treeDocument{}

	if err := yaml.Unmarshal(raw, &document); err != nil {
		cancel()
		return nil, errnie.Error(err)
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

	return tree, errnie.Error(errnie.Require((map[string]any{
		"ctx":          tree.ctx,
		"cancel":       tree.cancel,
		"branches":     tree.branches,
		"measurements": tree.Measurements,
	})))
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

	return tree, errnie.Error(errnie.Require(map[string]any{
		"ctx":      tree.ctx,
		"cancel":   tree.cancel,
		"branches": tree.branches,
	}))
}

func (tree *Tree) Action() *ActionType {
	return tree.currentAction
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
	tree.currentAction = nil
}

/*
Walk traverses the tree and returns the Action at the deepest reachable branch.
*/
func (tree *Tree) Walk(measurements []Measurement, branches ...Branch) *ActionType {
	return tree.WalkContext(BranchContext{Measurements: measurements}, branches...)
}

/*
WalkContext traverses the tree using the complete branch evaluation context.
*/
func (tree *Tree) WalkContext(
	branchContext BranchContext, branches ...Branch,
) *ActionType {
	evaluator := NewBranchEvaluator(branchContext)
	tree.currentAction = evaluator.Action(BranchList(branches))
	tree.err = evaluator.Err()

	if tree.err != nil {
		errnie.Error(tree.err)
	}

	return tree.currentAction
}
