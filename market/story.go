package market

import (
	"context"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
)

/*
Story holds the latest playbook verdicts per symbol for dashboards and audits.
*/
type Story struct {
	ctx      context.Context
	cancel   context.CancelFunc
	err      error
	pool     *qpool.Q[any]
	symbols  *sync.Map
	balances *logic.Balances
	tree     *logic.Tree
	traces   []logic.WalkTrace
}

func NewStory(
	ctx context.Context,
	pool *qpool.Q[any],
) *Story {
	ctx, cancel := context.WithCancel(ctx)

	tree, err := logic.NewTree(ctx, pool)

	if err != nil {
		cancel()
		errnie.Error(errnie.Err(
			errnie.Validation,
			"story: failed to create tree",
			err,
		))
	}

	story := &Story{
		ctx:     ctx,
		cancel:  cancel,
		pool:    pool,
		symbols: &sync.Map{},
		tree:    tree,
	}

	return story
}

/*
Update evaluates playbook verdicts for the given scope measurements against the
supplied holdings, so playbook conditions (e.g. symbolHeld) see the live ledger.
*/
func (story *Story) Update(
	measurements []*datura.Artifact,
	balances *logic.Balances,
) ([]*datura.Artifact, error) {
	if story == nil || len(measurements) == 0 {
		return nil, nil
	}

	story.balances = balances

	// Group measurements by symbol and walk the playbook once per symbol. The
	// playbook's stage state and conditions are per-symbol (branch.Evaluate keys
	// on the measurement scope), so a tick that carries many symbols must walk
	// each in isolation — evaluating the whole batch at once only ever sees the
	// first symbol. Each walk yields both the candidate action and the full
	// descent trace; the traces drive the Decision Tree surface.
	bySymbol := groupBySymbol(measurements)
	actions := make([]*logic.Action, 0, len(bySymbol))
	traces := make([]logic.WalkTrace, 0, len(bySymbol))

	for _, symbolMeasurements := range bySymbol {
		candidates, trace, walkErr := logic.WalkTreeActions(
			symbolFromArtifacts(symbolMeasurements),
			symbolMeasurements,
			story.balances,
			story.tree.Branches,
		)

		traces = append(traces, trace)

		if walkErr != nil {
			story.err = errnie.Error(errnie.Err(
				errnie.Validation,
				"story: playbook produced invalid action",
				walkErr,
			))

			return nil, story.err
		}

		// The playbook proposes every candidate it found for this symbol; the
		// trader ranks and chooses among them. Story does not collapse to one.
		actions = append(actions, candidates...)
	}

	story.traces = traces

	artifacts := make([]*datura.Artifact, 0)

	for _, action := range actions {
		buf, err := sonic.Marshal(action)

		if err != nil {
			story.err = errnie.Error(errnie.Err(
				errnie.Validation,
				"story: failed to marshal action",
				err,
			))

			return nil, story.err
		}

		artifact := datura.Acquire("story", datura.APPJSON)
		artifact.WithRole(string(action.Side))
		artifact.WithScope(action.Symbol)
		artifact.WithPayload(buf)
		artifacts = append(artifacts, artifact)
	}

	return artifacts, nil
}

/*
Traces returns the per-symbol playbook descent traces from the most recent
Update. The Decision Tree surface renders these: which branches matched, parked,
or rejected for each symbol, and the active path that produced a candidate.
*/
func (story *Story) Traces() []logic.WalkTrace {
	return story.traces
}

/*
groupBySymbol partitions a measurement batch by scope (symbol) so each symbol's
playbook walk sees only its own evidence. Order within a symbol is preserved.
*/
func groupBySymbol(measurements []*datura.Artifact) map[string][]*datura.Artifact {
	bySymbol := make(map[string][]*datura.Artifact)

	for _, measurement := range measurements {
		symbol, err := measurement.Scope()

		if err != nil || symbol == "" {
			continue
		}

		bySymbol[symbol] = append(bySymbol[symbol], measurement)
	}

	return bySymbol
}

/*
symbolFromArtifacts returns the scope shared by a per-symbol measurement group.
*/
func symbolFromArtifacts(measurements []*datura.Artifact) string {
	if len(measurements) == 0 {
		return ""
	}

	symbol, _ := measurements[0].Scope()

	return symbol
}

/*
Error returns the story's error.
*/
func (story *Story) Error() error {
	return story.err
}

/*
Close shuts down the story.
*/
func (story *Story) Close() error {
	story.cancel()
	return nil
}
