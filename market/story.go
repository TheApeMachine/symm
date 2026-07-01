package market

import (
	"context"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
)

/*
Story holds the latest playbook verdicts per symbol for dashboards and audits.
*/
type Story struct {
	ctx     context.Context
	cancel  context.CancelFunc
	err     error
	pool    *qpool.Q[any]
	symbols *sync.Map
	dirtyMu sync.Mutex
	dirty   map[string][]*datura.Artifact
	tree    *logic.Tree
}

type storyActionResult struct {
	symbol  string
	actions []*datura.Artifact
	trace   StoryTrace
}

type StoryTrace struct {
	Symbol                 string
	BranchesEvaluated      int
	BranchesMatched        int
	CandidateCount         int
	CandidateCountByBranch map[string]int
	FreshSources           map[string]int
	StaleSources           map[string]int
	MissingSources         map[string]int
	FirstBlocker           string
	Conditions             []logic.ConditionTrace
}

func NewStory(
	ctx context.Context,
	pool *qpool.Q[any],
) *Story {
	tree := errnie.Does(func() (*logic.Tree, error) {
		return logic.NewTree(ctx, pool)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			err.Error(),
			err,
		))
	}).Value()

	return NewStoryWithTree(ctx, pool, tree)
}

func NewStoryWithTree(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *logic.Tree,
) *Story {
	ctx, cancel := context.WithCancel(ctx)

	story := &Story{
		ctx:     ctx,
		cancel:  cancel,
		pool:    pool,
		symbols: &sync.Map{},
		dirty:   make(map[string][]*datura.Artifact),
		tree:    tree,
	}

	return story
}

/*
Update evaluates playbook verdicts for the given scope measurements against the
supplied holdings, so playbook conditions (e.g. symbolHeld) see the live ledger.
*/
func (story *Story) Update(measurements []*datura.Artifact) {
	if story.symbols == nil {
		story.symbols = &sync.Map{}
	}

	story.dirtyMu.Lock()
	if story.dirty == nil {
		story.dirty = make(map[string][]*datura.Artifact)
	}
	defer story.dirtyMu.Unlock()

	for _, measurement := range measurements {
		if measurement == nil {
			continue
		}

		symbol, err := measurement.Scope()
		if err != nil || symbol == "" {
			continue
		}

		ring, _ := story.symbols.LoadOrStore(
			symbol, structure.NewListRing[*datura.Artifact](64),
		)

		ring.(*structure.ListRing[*datura.Artifact]).Push(
			measurement,
		)
		story.dirty[symbol] = append(story.dirty[symbol], measurement)
	}
}

/*
Actions lazily evaluates the decision tree, and potentially generates
candidate actions, which are used by the trader as a mechanism to scope
down the measurements into something it can reason about and make choices.
*/
func (story *Story) Actions(balances *datura.Artifact) []*datura.Artifact {
	actions, _ := story.ActionsWithTrace(balances)
	return actions
}

func (story *Story) ActionsWithTrace(balances *datura.Artifact) ([]*datura.Artifact, []StoryTrace) {
	dirty := story.consumeDirty()
	if len(dirty) == 0 {
		return nil, nil
	}

	symbols := make([]string, 0, len(dirty))
	for symbol := range dirty {
		symbols = append(symbols, symbol)
	}
	sort.Strings(symbols)

	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	if workers > len(symbols) {
		workers = len(symbols)
	}

	jobs := make(chan string)
	results := make(chan storyActionResult, len(symbols))
	var wait sync.WaitGroup

	for range workers {
		wait.Go(func() {
			for symbol := range jobs {
				results <- story.actionsForSymbol(symbol, dirty[symbol], balances)
			}
		})
	}

	for _, symbol := range symbols {
		jobs <- symbol
	}
	close(jobs)
	wait.Wait()
	close(results)

	bySymbol := make(map[string][]*datura.Artifact, len(symbols))
	traceBySymbol := make(map[string]StoryTrace, len(symbols))
	for result := range results {
		bySymbol[result.symbol] = result.actions
		traceBySymbol[result.symbol] = result.trace
	}

	actions := make([]*datura.Artifact, 0)
	traces := make([]StoryTrace, 0, len(symbols))
	for _, symbol := range symbols {
		actions = append(actions, bySymbol[symbol]...)
		traces = append(traces, traceBySymbol[symbol])
	}

	return actions, traces
}

func (story *Story) actionsForSymbol(
	symbol string,
	updated []*datura.Artifact,
	balances *datura.Artifact,
) storyActionResult {
	result := storyActionResult{symbol: symbol}
	value, ok := story.symbols.Load(symbol)
	if !ok || story.tree == nil {
		return result
	}

	ring, _ := value.(*structure.ListRing[*datura.Artifact])
	measurements := make([]*datura.Artifact, 0)

	ring.Do(func(measurement *datura.Artifact) {
		if measurement == nil {
			return
		}

		measurements = append(measurements, measurement)
	})

	candidates, err := story.tree.Evaluate(
		symbol, measurements, balances, story.tree.Branches,
	)
	evaluationTrace := story.tree.EvaluateTrace(
		symbol, measurements, balances, story.tree.Branches,
	)

	if err != nil {
		errnie.Error(err)
	}

	result.trace = summarizeStoryTrace(symbol, evaluationTrace, candidates)

	for _, measurement := range updated {
		measurement.WithAttribute("journey.story.evaluated", true)
		measurement.WithAttribute("journey.story.candidates", len(candidates))
	}

	for _, candidate := range candidates {
		if candidate.DecisionID == "" {
			candidate.DecisionID = uuid.NewString()
		}
		if candidate.ActionID == "" {
			candidate.ActionID = uuid.NewString()
		}

		payload, err := sonic.Marshal(candidate)

		if err != nil {
			errnie.Error(err)
		}

		action := datura.Acquire(
			"story", datura.APPJSON,
		).WithPayload(
			payload,
		).WithRole(
			string(candidate.Side),
		).WithScope(
			candidate.Symbol,
		).WithAttribute(
			"journey.story.status", "candidate",
		).WithAttribute(
			"journey.story.symbol", candidate.Symbol,
		).WithAttribute(
			"journey.story.source", string(candidate.ReasonSource),
		).WithAttribute(
			"journey.story.category", string(candidate.ReasonCategory),
		).Poke(
			candidate.DecisionID, "decision_id",
		).Poke(
			candidate.ActionID, "action_id",
		).Poke(
			storyCandidateBranchKey(candidate), "branch_id",
		)

		result.actions = append(result.actions, action)
	}

	return result
}

func summarizeStoryTrace(
	symbol string,
	evaluationTrace logic.EvaluationTrace,
	candidates []*logic.Action,
) StoryTrace {
	trace := StoryTrace{
		Symbol:                 symbol,
		BranchesEvaluated:      evaluationTrace.BranchesEvaluated,
		BranchesMatched:        evaluationTrace.BranchesMatched,
		CandidateCount:         len(candidates),
		CandidateCountByBranch: make(map[string]int),
		FreshSources:           make(map[string]int),
		StaleSources:           make(map[string]int),
		MissingSources:         make(map[string]int),
		Conditions:             evaluationTrace.Conditions,
	}

	for _, candidate := range candidates {
		trace.CandidateCountByBranch[storyCandidateBranchKey(candidate)]++
	}

	for _, condition := range evaluationTrace.Conditions {
		source := strings.TrimSpace(string(condition.Source))
		if source == "" {
			source = "holding"
		}

		switch condition.Reason {
		case logic.TraceReasonStaleSource:
			trace.StaleSources[source]++
		case logic.TraceReasonMissingSource:
			trace.MissingSources[source]++
		case logic.TraceReasonWrongSymbol:
			trace.MissingSources[source+":wrong_symbol"]++
		default:
			trace.FreshSources[source]++
		}

		if condition.Result || trace.FirstBlocker != "" {
			continue
		}

		trace.FirstBlocker = storyBlockerLabel(condition)
	}

	return trace
}

func storyCandidateBranchKey(candidate *logic.Action) string {
	if candidate == nil {
		return "unknown"
	}

	source := strings.TrimSpace(string(candidate.ReasonSource))
	category := strings.TrimSpace(string(candidate.ReasonCategory))
	if source != "" && category != "" {
		return source + "." + category
	}
	if source != "" {
		return source
	}
	if category != "" {
		return category
	}

	return strings.TrimSpace(string(candidate.Type)) + "." + strings.TrimSpace(string(candidate.Side))
}

func storyBlockerLabel(condition logic.ConditionTrace) string {
	source := strings.TrimSpace(string(condition.Source))
	if source == "" {
		source = "holding"
	}

	parts := []string{source}
	if condition.Category != logic.CategoryTypeNone {
		parts = append(parts, string(condition.Category))
	}
	parts = append(parts, string(condition.Reason))

	return strings.Join(parts, ":")
}

func (story *Story) consumeDirty() map[string][]*datura.Artifact {
	story.dirtyMu.Lock()
	defer story.dirtyMu.Unlock()

	dirty := story.dirty
	story.dirty = make(map[string][]*datura.Artifact)

	if dirty == nil {
		return map[string][]*datura.Artifact{}
	}

	return dirty
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
