package optimizer

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	nomcausal "github.com/theapemachine/nomagique/causal"
	"github.com/theapemachine/nomagique/mcts"
)

const (
	vectorAction = iota
	vectorReward
	vectorDepth
	vectorTrades
	vectorDrawdown
	vectorPositions
)

func Optimize(baseTree []byte, frames []ReplayFrame, options Options) (Report, []byte, error) {
	options = normalizeOptions(options)
	if len(baseTree) == 0 {
		return Report{}, nil, fmt.Errorf("optimizer: empty playbook tree")
	}
	if err := validateFrames(frames); err != nil {
		return Report{}, nil, err
	}

	evaluator := NewReplayEvaluator(frames, options)
	return optimizeWithEvaluator(baseTree, frames, options, evaluator)
}

func optimizeWithEvaluator(
	baseTree []byte,
	frames []ReplayFrame,
	options Options,
	evaluator Evaluator,
) (Report, []byte, error) {
	options = normalizeOptions(options)
	options.Progressf("baseline replay: frames=%d", len(frames))
	started := time.Now()
	baseline, err := evaluator.Evaluate(baseTree)
	if err != nil {
		return Report{}, nil, err
	}
	options.Progressf(
		"baseline done: wallet=%.2f reward=%.4f trades=%d positions=%d elapsed=%s",
		baseline.Wallet,
		baseline.Reward,
		baseline.Trades,
		baseline.Positions,
		time.Since(started).Round(time.Millisecond),
	)

	cache := &evaluationCache{
		baseTree:  append([]byte(nil), baseTree...),
		evaluator: evaluator,
		plans:     make(map[string]evaluatedPlan),
		progressf: options.Progressf,
	}
	mutations := DefaultMutations()
	root := searchState{
		baseTree:   append([]byte(nil), baseTree...),
		mutations:  mutations,
		cache:      cache,
		maxDepth:   options.MaxDepth,
		reward:     baseline.Reward,
		result:     baseline,
		lastAction: 0,
	}

	search := mcts.NewCausalMCTS(
		nomcausal.NewMCTSAdapter(),
		options.Exploration,
		options.CausalAlpha,
		minRows(options),
		vectorAction,
		vectorReward,
		[]int{vectorDepth, vectorTrades, vectorDrawdown, vectorPositions},
		[]int{vectorAction, vectorReward, vectorDepth, vectorTrades, vectorDrawdown, vectorPositions},
		true,
	)

	options.Progressf(
		"mcts search: iterations=%d max_depth=%d mutations=%d",
		options.Iterations,
		options.MaxDepth,
		len(mutations),
	)
	started = time.Now()
	_, err = search.Search(root, options.Iterations, nil)
	if err != nil {
		return Report{}, nil, err
	}
	options.Progressf("mcts done: evaluated=%d elapsed=%s", len(cache.Plans()), time.Since(started).Round(time.Millisecond))

	plans := cache.Plans()
	best := evaluatedPlan{
		result: baseline,
		tree:   append([]byte(nil), baseTree...),
	}
	for _, plan := range plans {
		if plan.result.Reward > best.result.Reward {
			best = plan
		}
	}

	recommendations := make([]PlanReport, 0, len(plans)+1)
	recommendations = append(recommendations, planReport(nil, baseline))
	for _, plan := range plans {
		recommendations = append(recommendations, planReport(plan.names, plan.result))
	}
	sort.SliceStable(recommendations, func(first, second int) bool {
		return recommendations[first].Reward > recommendations[second].Reward
	})
	if len(recommendations) > 12 {
		recommendations = recommendations[:12]
	}

	report := Report{
		Frames:          len(frames),
		Symbols:         countSymbols(frames),
		Iterations:      options.Iterations,
		MaxDepth:        options.MaxDepth,
		PlansEvaluated:  len(plans),
		Baseline:        planReport(nil, baseline),
		Best:            planReport(best.names, best.result),
		Recommendations: recommendations,
	}

	return report, best.tree, nil
}

func normalizeOptions(options Options) Options {
	if options.Iterations <= 0 {
		options.Iterations = 80
	}
	if options.MaxDepth <= 0 {
		options.MaxDepth = 2
	}
	if options.Exploration <= 0 {
		options.Exploration = 1.4
	}
	if options.CausalAlpha <= 0 {
		options.CausalAlpha = 0.75
	}
	if options.InitialCash <= 0 {
		options.InitialCash = 200
	}
	if options.FeeRate <= 0 {
		options.FeeRate = 0.004
	}
	if options.MakerFeeRate <= 0 {
		options.MakerFeeRate = 0.0025
	}
	if options.MaxPositions <= 0 {
		options.MaxPositions = 3
	}
	if options.Progressf == nil {
		options.Progressf = func(string, ...any) {}
	}

	return options
}

func minRows(options Options) int {
	rows := int(math.Ceil(math.Sqrt(float64(options.Iterations))))
	if rows < 6 {
		return 6
	}

	return rows
}

func planReport(names []string, result ReplayResult) PlanReport {
	if len(names) == 0 {
		names = []string{"baseline"}
	}

	return PlanReport{
		Mutations:   append([]string(nil), names...),
		Reward:      result.Reward,
		Wallet:      result.Wallet,
		Cash:        result.Cash,
		Trades:      result.Trades,
		Positions:   result.Positions,
		MaxDrawdown: result.MaxDrawdown,
		StartedAt:   result.StartedAt,
		EndedAt:     result.EndedAt,
	}
}

type searchState struct {
	baseTree   []byte
	mutations  []Mutation
	selected   []int
	names      []string
	cache      *evaluationCache
	maxDepth   int
	reward     float64
	result     ReplayResult
	lastAction float64
}

func (state searchState) IsTerminal() bool {
	return len(state.selected) >= state.maxDepth || len(state.GetPossibleActions()) == 0
}

func (state searchState) GetReward() float64 {
	return state.reward
}

func (state searchState) GetPossibleActions() []float64 {
	if len(state.selected) >= state.maxDepth {
		return nil
	}

	seen := make(map[int]struct{}, len(state.selected))
	for _, index := range state.selected {
		seen[index] = struct{}{}
	}

	actions := make([]float64, 0, len(state.mutations)-len(seen))
	for index, mutation := range state.mutations {
		if _, ok := seen[index]; ok {
			continue
		}
		actions = append(actions, mutation.ID)
	}

	return actions
}

func (state searchState) ApplyAction(action float64) mcts.State {
	index := -1
	for candidateIndex, mutation := range state.mutations {
		if mutation.ID == action {
			index = candidateIndex
			break
		}
	}
	if index < 0 {
		return state
	}

	nextSelected := append(append([]int(nil), state.selected...), index)
	plan, err := state.cache.Evaluate(nextSelected, state.mutations)
	if err != nil {
		return searchState{
			baseTree:   state.baseTree,
			mutations:  state.mutations,
			selected:   nextSelected,
			names:      mutationNames(nextSelected, state.mutations),
			cache:      state.cache,
			maxDepth:   state.maxDepth,
			reward:     math.Inf(-1),
			lastAction: action,
		}
	}

	return searchState{
		baseTree:   state.baseTree,
		mutations:  state.mutations,
		selected:   nextSelected,
		names:      plan.names,
		cache:      state.cache,
		maxDepth:   state.maxDepth,
		reward:     plan.result.Reward,
		result:     plan.result,
		lastAction: action,
	}
}

func (state searchState) ToVector() []float64 {
	return []float64{
		state.lastAction,
		state.reward,
		float64(len(state.selected)),
		float64(state.result.Trades),
		state.result.MaxDrawdown,
		float64(state.result.Positions),
	}
}

type evaluatedPlan struct {
	names  []string
	result ReplayResult
	tree   []byte
}

type evaluationCache struct {
	mu        sync.Mutex
	baseTree  []byte
	evaluator Evaluator
	plans     map[string]evaluatedPlan
	progressf ProgressFunc
}

func (cache *evaluationCache) Evaluate(selected []int, mutations []Mutation) (evaluatedPlan, error) {
	key := selectedKey(selected)

	cache.mu.Lock()
	if plan, ok := cache.plans[key]; ok {
		cache.mu.Unlock()
		return plan, nil
	}
	cache.mu.Unlock()

	treeBytes := append([]byte(nil), cache.baseTree...)
	for _, index := range selected {
		next, err := mutations[index].Apply(treeBytes)
		if err != nil {
			return evaluatedPlan{}, err
		}
		treeBytes = next
	}

	names := mutationNames(selected, mutations)
	started := time.Now()
	cache.progressf("replay plan: %s", strings.Join(names, " + "))
	result, err := cache.evaluator.Evaluate(treeBytes)
	if err != nil {
		return evaluatedPlan{}, err
	}
	cache.progressf(
		"plan done: %s wallet=%.2f reward=%.4f trades=%d positions=%d elapsed=%s",
		strings.Join(names, " + "),
		result.Wallet,
		result.Reward,
		result.Trades,
		result.Positions,
		time.Since(started).Round(time.Millisecond),
	)

	plan := evaluatedPlan{
		names:  names,
		result: result,
		tree:   treeBytes,
	}

	cache.mu.Lock()
	cache.plans[key] = plan
	cache.mu.Unlock()

	return plan, nil
}

func (cache *evaluationCache) Plans() []evaluatedPlan {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	plans := make([]evaluatedPlan, 0, len(cache.plans))
	for _, plan := range cache.plans {
		plans = append(plans, plan)
	}

	return plans
}

func selectedKey(selected []int) string {
	if len(selected) == 0 {
		return ""
	}

	parts := make([]string, len(selected))
	for index, value := range selected {
		parts[index] = fmt.Sprintf("%d", value)
	}

	return strings.Join(parts, "/")
}

func mutationNames(selected []int, mutations []Mutation) []string {
	names := make([]string, 0, len(selected))
	for _, index := range selected {
		if index >= 0 && index < len(mutations) {
			names = append(names, mutations[index].Name)
		}
	}

	return names
}

func countSymbols(frames []ReplayFrame) int {
	symbols := make(map[string]struct{})
	for _, frame := range frames {
		for symbol := range frame.Prices {
			symbols[symbol] = struct{}{}
		}
	}

	return len(symbols)
}
