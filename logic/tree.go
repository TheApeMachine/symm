package logic

import (
	"embed"
	"strconv"

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
	return tree.EvaluateTraced(measurements, holdings, nil)
}

/*
EvaluateTraced runs the playbook and optionally records the gate path for audit.
Entry-path traces are collected per branch; the deepest blocking gate wins.
*/
func (tree *Tree) EvaluateTraced(
	measurements []Measurement,
	holdings *Holdings,
	trace *EvalTrace,
) *Evaluation {
	evalContext := NewEvalContext(measurements, holdings)

	if tree.stats != nil {
		tree.stats.BeginEvaluation()
	}

	bestDepth := 0
	bestBranchIndex := 999
	var bestAudit *EvalTrace

	for branchIndex, branch := range tree.Branches {
		branchKey := strconv.Itoa(branchIndex)
		branchTrace := traceForBranch(trace, branchIndex)

		evaluation, err := branch.evaluate(
			measurements,
			evalContext,
			tree.stats,
			branchTrace,
			branchKey,
		)

		if errnie.Error(err) != nil {
			continue
		}

		if evaluation != nil {
			if trace != nil && branchTrace != nil {
				trace.Adopt(branchTrace)
			}

			return evaluation
		}

		if trace == nil || branchIndex < firstEntryBranchIndex || branchTrace == nil {
			continue
		}

		if bestAudit == nil || branchTrace.BeatsAuditScore(bestDepth, bestBranchIndex) {
			bestAudit = branchTrace
			bestDepth, bestBranchIndex = branchTrace.AuditScore()
		}
	}

	if trace != nil && bestAudit != nil {
		trace.Adopt(bestAudit)
	}

	return nil
}

func traceForBranch(auditTrace *EvalTrace, branchIndex int) *EvalTrace {
	if auditTrace == nil {
		return nil
	}

	return &EvalTrace{}
}

/*
Stats exposes playbook instrumentation for dashboard publishing.
*/
func (tree *Tree) Stats() *TreeStats {
	return tree.stats
}
