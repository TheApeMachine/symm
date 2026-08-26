package cognition

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
Option configures the Cognition solver.
*/
type Option func(*Solver)

/*
WithMaxSequenceLength sets the maximum token depth before a sequence naturally terminates.
*/
func WithMaxSequenceLength(length int) Option {
	return func(s *Solver) {
		s.maxSeqLen = length
	}
}

/*
WithSurprisalLimit sets the information-theoretic threshold (in bits) that triggers
a sequence break when an unexpected category transition occurs.
*/
func WithSurprisalLimit(limitBits float64) Option {
	return func(s *Solver) {
		s.surprisalLimit = limitBits
	}
}

/*
WithBeamShape sets the lookahead beam width and hop count.
*/
func WithBeamShape(width, hops int) Option {
	return func(s *Solver) {
		s.beamWidth = width
		s.maxHops = hops
	}
}

/*
WithPrefixTreeShape bounds the sensory prefix tree exported for display: how many
continuations are drawn per node, how deep the drawing walks, and the hard node
ceiling that keeps a wide trie from flooding a reading.
*/
func WithPrefixTreeShape(width, depth, maxNodes int) Option {
	return func(s *Solver) {
		s.branchWidth = width
		s.branchDepth = depth
		s.maxBranchNodes = maxNodes
	}
}

type symbolCognitionState struct {
	activeTokens  []string
	activeRegime  types.Category
	reading       types.Cognition
	hasReading    bool
	branches      []types.CognitionBranch
	branchesStamp uint64
	classScratch  dmt.ClassificationScratch
	beamScratch   dmt.BeamSearchScratch
}

/*
Solver uses dmt.Tree to learn, score, and forecast market category transition sequences,
classify macro regimes via attractor basins, and predict future category paths using beam search.
*/
type Solver struct {
	ctx            context.Context
	cancel         context.CancelFunc
	err            error
	thesis         *types.Thesis
	recorder       *audit.Recorder
	treeMu         sync.RWMutex
	tree           *dmt.Tree
	states         sync.Map // string (symbol) -> *symbolCognitionState
	maxSeqLen      int
	surprisalLimit float64
	tickCounter    atomic.Uint64
	ObserveModule  func(string, time.Duration)

	// Beam search shape. Held as fields rather than call-site constants so the
	// lookahead can be tuned without also reshaping what Cortex draws — the two
	// were previously the same two numbers doing both jobs.
	beamWidth int
	maxHops   int

	// Prefix tree render shape. These bound what is exported for display only;
	// they never gate learning or search.
	branchWidth    int
	branchDepth    int
	maxBranchNodes int

	// Consolidation products, refreshed on the REM schedule rather than per
	// tick, because they read weights only consolidation rewrites.
	dreams  []string
	symbols []types.CognitionSymbol

	// remOutcome is the most recent consolidation pass's own report — replay
	// count, decay factor, and how much of the sensory namespace retroactive
	// inhibition pruned. remFrom/remThrough are the window that pass covered.
	// remOutcome stays what it was between passes, since consolidation runs
	// far less often than a tick, so a reading between passes reports the
	// last real consolidation rather than a manufactured zero.
	remOutcome dmt.REMConsolidationOutcome
	remFrom    time.Time
	remThrough time.Time
}

const categoryTokenSeparator = "\x1f"

func (solver *Solver) getSymbolState(symbol string) *symbolCognitionState {
	loaded, found := solver.states.Load(symbol)

	if found {
		return loaded.(*symbolCognitionState)
	}

	candidate := &symbolCognitionState{
		beamScratch: dmt.BeamSearchScratch{
			CurrentBeams: make([]dmt.BeamPath, 0, 4),
			NextBeams:    make([]dmt.BeamPath, 0, 4),
			LookupBuffer: make([]dmt.LookaheadPrediction, 0, 8),
		},
	}
	actual, _ := solver.states.LoadOrStore(symbol, candidate)

	return actual.(*symbolCognitionState)
}

/*
NewSolver returns a new cognition solver bound to a radix tree.
*/
func NewSolver(
	ctx context.Context,
	bus *runtime.Workspace,
	opts ...Option,
) *Solver {
	ctx, cancel := context.WithCancel(ctx)

	var thesis *types.Thesis
	var tree *dmt.Tree
	if bus != nil {
		if shared, found := bus.Shared("thesis", ""); found {
			if t, ok := shared.(*types.Thesis); ok {
				thesis = t
			}
		}
		if shared, found := bus.Shared("tree", ""); found {
			if t, ok := shared.(*dmt.Tree); ok {
				tree = t
			}
		}
	}

	if tree == nil {
		tree, _ = dmt.NewTree("")
	}

	solver := &Solver{
		ctx:            ctx,
		cancel:         cancel,
		thesis:         thesis,
		tree:           tree,
		maxSeqLen:      6,   // Max 6 category transitions per sequence window
		surprisalLimit: 3.5, // > 3.5 bits surprisal (P < 8.8%) indicates a regime break
		beamWidth:      3,
		maxHops:        2,
		branchWidth:    4,
		branchDepth:    5,
		maxBranchNodes: 192,
	}

	for _, opt := range opts {
		opt(solver)
	}

	if bus != nil {
		runtime.WireFunc[[]types.Category, *types.Cognition](
			bus,
			types.ChannelCategories,
			types.ChannelCognition,
			solver.Step,
		)
	}

	return solver
}

func (solver *Solver) Name() string {
	return "cognition"
}

func (solver *Solver) Error() error { return solver.err }

// Step folds one category batch into the symbol's cognition state machine and
// returns the freshest reading for downstream subscribers.
func (solver *Solver) Step(categories []types.Category) *types.Cognition {
	if len(categories) == 0 {
		return nil
	}

	started := time.Now()
	defer func() {
		if solver.ObserveModule != nil {
			solver.ObserveModule("cognition", time.Since(started))
		}
	}()

	config := system.Cfg.Snapshot()
	switchThreshold := config.Planner.MinimumConfidence
	rows := make(map[string]types.Cognition, 1)

	if err := solver.processBatch(
		categories[0].Symbol,
		categories,
		switchThreshold,
		rows,
	); err != nil {
		solver.err = errnie.Error(err)
		return nil
	}

	if reading, ok := rows[categories[0].Symbol]; ok {
		return &reading
	}

	return nil
}

/*
processBatch advances one symbol's cognition state machine from a single
category batch, publishing its reading to the downstream Cognition output.
*/
func (solver *Solver) processBatch(
	symbol string,
	categories []types.Category,
	switchThreshold float64,
	rows map[string]types.Cognition,
) error {
	symbolState, found := solver.thesis.Symbols.Load(symbol)

	if !found || symbolState == nil {
		return nil
	}

	state := solver.getSymbolState(symbol)

	// Select the dominant category for this symbol on this observation.
	dominantCategory := solver.selectDominantCategory(categories)

	if dominantCategory == types.CategoryTypeNone {
		return nil
	}

	categoryToken := solver.encodeCategory(dominantCategory)
	observedRegime := solver.selectRegime(categories)
	activeTokens := state.activeTokens
	activeRegime := state.activeRegime
	transitioned := len(activeTokens) == 0 ||
		activeTokens[len(activeTokens)-1] != categoryToken

	if !transitioned {
		return nil
	}

	if len(rows) == 0 {
		solver.tickCounter.Add(1)
	}

	// 2. Evaluate if appending this category causes a Sequence Break
	broken, _ := solver.evalSequenceBreak(activeTokens, categoryToken)

	if broken && len(activeTokens) > 0 {
		// --- SEQUENCE BREAK DETECTED ---
		oldSequenceBytes := solver.sequenceBytes(activeTokens)

		// Commit completed sequence to episodic buffer for REM replay
		solver.treeMu.Lock()
		_, _ = solver.tree.CommitToEpisodicBuffer(
			uint64(solver.thesis.At.UnixNano()), oldSequenceBytes,
		)

		if activeRegime.Type != types.CategoryTypeNone {
			err := solver.tree.TeachSequence(
				oldSequenceBytes, []byte(regimeName(activeRegime.Type)),
			)

			if err != nil {
				solver.treeMu.Unlock()
				return errnie.Error(errnie.Err(
					errnie.Internal,
					fmt.Sprintf(
						"cognition: failed to learn %s regime for %s",
						regimeName(activeRegime.Type), symbol,
					),
					err,
				))
			}
		}
		solver.treeMu.Unlock()

		// Start fresh sequence buffer with new category
		activeTokens = []string{categoryToken}

		if observedRegime.Type != types.CategoryTypeNone {
			activeRegime = observedRegime
		}
	} else {
		// --- SEQUENCE CONTINUES ---
		activeTokens = append(activeTokens, categoryToken)

		if observedRegime.Type != types.CategoryTypeNone {
			activeRegime = observedRegime
		}
	}

	state.activeTokens = activeTokens
	state.activeRegime = activeRegime

	activeSequenceBytes := solver.sequenceBytes(activeTokens)

	solver.treeMu.RLock()
	// 4. Classify macro market regime / concept attractor basin
	classResult := solver.tree.Classify(activeSequenceBytes, &state.classScratch)

	// 5. Lookahead Beam Search: Predict next 2-3 likely category hops
	beamPaths := solver.tree.ExecuteBeamSearch(
		activeSequenceBytes,
		solver.beamWidth,
		solver.maxHops,
		&state.beamScratch,
	)

	/*
		6. Measure Branch Ambiguity (Shannon Entropy)

		MeasureBranchAmbiguity seeks the storage key verbatim — unlike
		GetSensoryWeight and PredictNextSensoryTokens, which namespace their
		argument internally. Handing it a bare sequence searched a namespace
		nothing is written to, so every prefix reported zero bits and the
		entropy gate could never open.
	*/
	ambiguity := solver.tree.MeasureBranchAmbiguity(
		dmt.SensoryPrefixKey(activeSequenceBytes),
	)
	solver.treeMu.RUnlock()

	var entropyBits *float64
	var entropyThreshold *float64

	if ambiguity.Threshold < math.MaxFloat64 {
		entropyBits = &ambiguity.EntropyBits
		entropyThreshold = &ambiguity.Threshold
	}

	// 7. Format Lookahead Predictions for Thesis
	predictions := solver.formatLookaheadPredictions(
		beamPaths, activeSequenceBytes,
	)

	classes := make([]types.CognitionClass, 0, len(classResult.Scores))

	for _, score := range classResult.Scores {
		if !isRegimeName(string(score.ClassName)) {
			continue
		}

		classes = append(classes, types.CognitionClass{
			Name:        string(score.ClassName),
			Probability: score.Value,
		})
	}

	beams := make([]types.CognitionBeam, 0, len(beamPaths))

	for _, path := range beamPaths {
		beams = append(beams, types.CognitionBeam{
			Sequence: solver.decodeCategoryPath(path.Sequence),
			Key:      string(path.Sequence),
			Score:    path.Score,
		})
	}

	branches := solver.cachedPrefixTree(state, activeTokens, transitioned)

	contrast := 0.0
	contrastEvidence := 0.0

	if len(classResult.Scores) > 1 {
		contrast = classResult.Scores[0].Value - classResult.Scores[1].Value
		solver.treeMu.RLock()
		evidence := solver.tree.ComputeBasinContrastiveEvidence(
			classResult.Scores[0].ClassName,
			classResult.Scores[1].ClassName,
			activeSequenceBytes,
		)
		solver.treeMu.RUnlock()
		contrastEvidence = evidence.Divergence
	}

	lookaheadScore := 0.0

	if len(beamPaths) > 0 {
		lookaheadScore = beamPaths[0].Score
	}

	winner := string(classResult.Winner)
	confidence := classResult.Highest
	classificationReady := len(classes) > 1 && isRegimeName(winner)

	if !classificationReady && activeRegime.Type != types.CategoryTypeNone {
		winner = regimeName(activeRegime.Type)
		confidence = activeRegime.Confidence
		classes = solver.regimeClasses(categories)
	}

	candidateWinner := winner
	candidateConfidence := confidence
	stabilized := solver.stabilizeReading(
		state,
		candidateWinner,
		candidateConfidence,
		ambiguity.Ambiguous,
		classes,
		predictions,
		switchThreshold,
	)
	winner = stabilized.winner
	confidence = stabilized.confidence
	predictions = stabilized.predictions

	solver.treeMu.RLock()
	analysis := solver.tree.AnalyzeInterpolated(activeSequenceBytes)
	sensoryWeight := solver.tree.GetSensoryWeight(activeSequenceBytes)
	solver.treeMu.RUnlock()

	contributions := make([]types.CognitionContribution, 0, len(analysis.Contributions))

	for _, contribution := range analysis.Contributions {
		contributions = append(contributions, types.CognitionContribution{
			Token: solver.decodeCategoryToken(string(contribution.Token)),
			Bits:  contribution.Bits,
		})
	}

	if len(contributions) == 0 {
		contributions = nil
	}

	cognition := types.Cognition{
		Source:           "cognition",
		Symbol:           symbol,
		At:               solver.thesis.At,
		Sequence:         solver.decodeCategoryPath(activeSequenceBytes),
		RegimePrefix:     winner,
		Winner:           winner,
		WinnerClass:      winner,
		CandidateWinner:  candidateWinner,
		StateHeld:        stabilized.held,
		PredictionsHeld:  stabilized.held,
		SwitchConfidence: candidateConfidence,
		SwitchThreshold:  switchThreshold,
		Confidence:       confidence,
		ClassConfidence:  confidence,
		Contrast:         contrast,
		ContrastEvidence: contrastEvidence,
		EntropyBits:      entropyBits,
		EntropyThreshold: entropyThreshold,
		Ambiguous:        ambiguity.Ambiguous,
		Cohort:           sensoryWeight.Count,
		LookaheadScore:   lookaheadScore,
		LookaheadPaths:   len(beamPaths),
		BeamWidth:        solver.beamWidth,
		MaxHops:          solver.maxHops,
		NodeCount:        len(branches),
		Predictions:      predictions,
		Branches:         branches,
		Beams:            beams,
		Classes:          classes,

		InterpolatedSurprisal: analysis.AverageSurprisal,
		Contributions:         contributions,
		Lexical:               solver.lexical(activeTokens),
		Symbols:               solver.symbols,
		Dreams:                solver.dreams,
		REMFrom:               solver.remFrom,
		REMThrough:            solver.remThrough,
		REMReplays:            int(solver.remOutcome.ReplayedObservations),
		REMDecayFactor:        solver.remOutcome.DecayFactor,
		REMInhibitionPct:      solver.remOutcome.RetroactiveInhibitionPct,
		// A pass runs synchronously inline on the 128-tick schedule
		// below, so "consolidating" is true only for the reading
		// published on the very tick that triggered it — every other
		// tick reports the awake state between passes.
		REMConsolidating: false,
	}

	state.reading = cognition
	state.hasReading = true
	rows[symbol] = cognition
	return nil
}

type stabilizedReading struct {
	winner      string
	confidence  float64
	predictions map[string]float64
	held        bool
}

/*
stabilizeReading keeps the accepted regime until an opposing posterior clears
the caller-supplied switch boundary. A held regime is display state only: its
lookahead is suppressed so persistence cannot manufacture predictive evidence
for the graph or planner.
*/
func (solver *Solver) stabilizeReading(
	state *symbolCognitionState,
	candidate string,
	candidateConfidence float64,
	ambiguous bool,
	classes []types.CognitionClass,
	predictions map[string]float64,
	switchThreshold float64,
) stabilizedReading {
	previous := state.reading
	hasPrevious := state.hasReading

	if !hasPrevious || previous.Winner == "" {
		return stabilizedReading{
			winner:      candidate,
			confidence:  candidateConfidence,
			predictions: predictions,
		}
	}

	if candidate == previous.Winner && !ambiguous {
		return stabilizedReading{
			winner:      candidate,
			confidence:  candidateConfidence,
			predictions: predictions,
		}
	}

	if candidate != "" && isRegimeName(candidate) && !ambiguous &&
		candidateConfidence >= switchThreshold {
		return stabilizedReading{
			winner:      candidate,
			confidence:  candidateConfidence,
			predictions: predictions,
		}
	}

	confidence, found := cognitionClassProbability(classes, previous.Winner)

	if !found {
		confidence = previous.Confidence
	}

	return stabilizedReading{
		winner:     previous.Winner,
		confidence: confidence,
		held:       true,
	}
}

func cognitionClassProbability(
	classes []types.CognitionClass,
	name string,
) (float64, bool) {
	for _, class := range classes {
		if class.Name == name {
			return class.Probability, true
		}
	}

	return 0, false
}

/*
lexical reports any active token that had to be resolved onto a different known
token, so a reading can show it was scored against a neighbour rather than
against itself.
*/
func (solver *Solver) lexical(activeTokens []string) []types.CognitionLexical {
	resolved := make([]types.CognitionLexical, 0, len(activeTokens))

	for _, token := range activeTokens {
		match := solver.tree.ResolveToken([]byte(token))

		if string(match.Mapped) == token {
			continue
		}

		resolved = append(resolved, types.CognitionLexical{
			Original:   solver.decodeCategoryToken(token),
			Mapped:     solver.decodeCategoryToken(string(match.Mapped)),
			Similarity: match.Similarity,
		})
	}

	if len(resolved) == 0 {
		return nil
	}

	return resolved
}

/*
evalSequenceBreak tests whether appending categoryToken causes a sequence break.
Returns true if surprisal exceeds limit or sequence length exceeds max.
*/
func (solver *Solver) evalSequenceBreak(
	activeTokens []string, nextToken string,
) (broken bool, surprisal float64) {
	if len(activeTokens) == 0 {
		return false, 0.0
	}

	if len(activeTokens) >= solver.maxSeqLen {
		return true, 0.0 // Sequence too long; force fresh break
	}

	// Test candidate path surprisal
	candidateTokens := append(append([]string(nil), activeTokens...), nextToken)
	candidateBytes := solver.sequenceBytes(candidateTokens)

	surprisalItems := solver.tree.InterpolatedSurprisal(candidateBytes)

	if len(surprisalItems) == 0 {
		return false, 0.0
	}

	// Check surprisal of the newly appended token
	lastItem := surprisalItems[len(surprisalItems)-1]
	surprisal = lastItem.Surprisal

	if surprisal >= solver.surprisalLimit {
		return true, surprisal // Unexpected transition -> Break Sequence!
	}

	return false, surprisal
}

/*
selectDominantCategory picks the category with the highest confidence/strength.
*/
func (solver *Solver) selectDominantCategory(
	categories []types.Category,
) types.CategoryType {
	if len(categories) == 0 {
		return types.CategoryTypeNone
	}

	best := categories[0]

	for _, cat := range categories[1:] {
		if cat.Confidence*cat.Strength > best.Confidence*best.Strength {
			best = cat
		}
	}

	return best.Type
}

/*
selectRegime resolves the strongest named regime-radar axis carried by the
current category evidence. Each axis is already normalized and classified by
the category stage, so cognition neither rescales it nor invents a second
classifier.
*/
func (solver *Solver) selectRegime(
	categories []types.Category,
) types.Category {
	regime := types.Category{}
	evidence := 0.0

	for _, category := range categories {
		categoryEvidence := category.Confidence * category.Strength

		if !isRegimeCategory(category.Type) || categoryEvidence <= evidence {
			continue
		}

		regime = category
		evidence = categoryEvidence
	}

	return regime
}

func isRegimeCategory(category types.CategoryType) bool {
	switch category {
	case types.CategoryTurbulent,
		types.CategoryOrganicTrend,
		types.CategoryAggressiveDrive,
		types.CategoryVolumeStarvation,
		types.CategoryStochasticBalance:
		return true
	default:
		return false
	}
}

func regimeName(category types.CategoryType) string {
	switch category {
	case types.CategoryTurbulent:
		return "volatility"
	case types.CategoryOrganicTrend:
		return "trend"
	case types.CategoryAggressiveDrive:
		return "drive"
	case types.CategoryVolumeStarvation:
		return "starved"
	case types.CategoryStochasticBalance:
		return "chop"
	default:
		return ""
	}
}

func isRegimeName(name string) bool {
	switch name {
	case "volatility", "trend", "drive", "starved", "chop":
		return true
	default:
		return false
	}
}

func (solver *Solver) regimeClasses(
	categories []types.Category,
) []types.CognitionClass {
	classes := make([]types.CognitionClass, 0, len(categories))

	for _, category := range categories {
		if !isRegimeCategory(category.Type) {
			continue
		}

		classes = append(classes, types.CognitionClass{
			Name:        regimeName(category.Type),
			Probability: category.Confidence,
		})
	}

	sort.SliceStable(classes, func(left, right int) bool {
		return classes[left].Probability > classes[right].Probability
	})

	return classes
}

/*
encodeCategory keeps one category as one DMT token.

DMT uses underscore as its sequence boundary, while category names themselves
contain underscores. Passing raw category names therefore trained
"vertical_ignition" as two unrelated states. Unit Separator is not used by any
category name and survives the byte-oriented tree unchanged.
*/
func (solver *Solver) encodeCategory(category types.CategoryType) string {
	return strings.ReplaceAll(string(category), "_", categoryTokenSeparator)
}

func (solver *Solver) decodeCategoryToken(token string) string {
	return strings.ReplaceAll(token, categoryTokenSeparator, "_")
}

/*
branchRefreshTicks bounds how stale a cached prefix tree may get when a symbol
sits in one category for a long stretch, so consolidation's rewrites still reach
the display eventually.
*/
const branchRefreshTicks = 256

/*
cachedPrefixTree returns the exported prefix tree for a symbol, walking the trie
only when that symbol's sequence moved or the cache has gone stale.
*/
func (solver *Solver) cachedPrefixTree(
	state *symbolCognitionState, activeTokens []string, transitioned bool,
) []types.CognitionBranch {
	cached := state.branches
	currentTick := solver.tickCounter.Load()
	stale := currentTick-state.branchesStamp >= branchRefreshTicks

	if len(cached) > 0 && !transitioned && !stale {
		return cached
	}

	solver.treeMu.RLock()
	observed := solver.prefixTreeBranches(activeTokens)
	solver.treeMu.RUnlock()

	if len(cached) == 0 {
		state.branches = observed
		state.branchesStamp = currentTick

		return observed
	}

	byKey := make(map[string]int, len(cached))

	for index, branch := range cached {
		byKey[branch.Key] = index
	}

	observedByID := make(map[int]types.CognitionBranch, len(observed))

	for _, branch := range observed {
		observedByID[branch.ID] = branch
		index, retained := byKey[branch.Key]

		if retained {
			branch.ID = cached[index].ID
			branch.ParentID = cached[index].ParentID
			cached[index] = branch
			continue
		}

		if branch.ParentID < 0 || len(cached) >= solver.maxBranchNodes {
			continue
		}

		parent := observedByID[branch.ParentID]
		parentIndex, parentRetained := byKey[parent.Key]

		if !parentRetained {
			continue
		}

		branch.ID = len(cached)
		branch.ParentID = cached[parentIndex].ID
		byKey[branch.Key] = len(cached)
		cached = append(cached, branch)
	}

	state.branches = cached
	state.branchesStamp = currentTick

	return cached
}

/*
branchFrontier is one pending node in the prefix tree walk.
*/
type branchFrontier struct {
	tokens []string
	id     int
	depth  int
}

/*
prefixTreeBranches exports the sensory prefix tree as Cortex draws it.

The walk enumerates each node's real continuations out of the radix tree rather
than reconstructing a path from the active sequence: siblings are what make the
structure a tree, and a projection built from one sequence can only ever emit a
spine no matter how much the tree holds.

The active sequence is pinned into the walk so the MAP beam always has a node to
highlight, even where its continuation is not among the strongest.
*/
func (solver *Solver) prefixTreeBranches(
	activeTokens []string,
) []types.CognitionBranch {
	branches := []types.CognitionBranch{{
		ID:          0,
		ParentID:    -1,
		Token:       "•",
		Probability: 1,
	}}

	frontier := []branchFrontier{{id: 0}}
	var lookahead [32]dmt.LookaheadPrediction

	for len(frontier) > 0 && len(branches) < solver.maxBranchNodes {
		node := frontier[0]
		frontier = frontier[1:]

		if node.depth >= solver.branchDepth {
			continue
		}

		tokens := solver.childTokens(node, activeTokens, lookahead[:0])

		for _, token := range tokens {
			if len(branches) >= solver.maxBranchNodes {
				break
			}

			childTokens := make([]string, 0, len(node.tokens)+1)
			childTokens = append(childTokens, node.tokens...)
			childTokens = append(childTokens, token)

			childBytes := solver.sequenceBytes(childTokens)
			weight := solver.tree.GetSensoryWeight(childBytes)
			childID := len(branches)

			branches = append(branches, types.CognitionBranch{
				ID:          childID,
				ParentID:    node.id,
				Token:       solver.decodeCategoryToken(token),
				Prefix:      solver.decodeCategoryPath(childBytes),
				Key:         string(childBytes),
				Depth:       node.depth + 1,
				Probability: weight.Probability,
				Count:       weight.Count,
			})

			frontier = append(frontier, branchFrontier{
				tokens: childTokens,
				id:     childID,
				depth:  node.depth + 1,
			})
		}
	}

	return branches
}

/*
childTokens returns the continuations drawn under one node, strongest first and
capped at the render width, with the active sequence's own continuation pinned in.
The returned tokens are copied out because the prediction buffer is reused.
*/
func (solver *Solver) childTokens(
	node branchFrontier,
	activeTokens []string,
	buffer []dmt.LookaheadPrediction,
) []string {
	predictions := solver.tree.PredictNextSensoryTokens(
		solver.sequenceBytes(node.tokens), buffer,
	)

	candidates := make([]dmt.LookaheadPrediction, len(predictions))
	copy(candidates, predictions)

	sort.SliceStable(candidates, func(left, right int) bool {
		return candidates[left].Probability > candidates[right].Probability
	})

	tokens := make([]string, 0, solver.branchWidth+1)

	for _, candidate := range candidates {
		if len(tokens) >= solver.branchWidth {
			break
		}

		tokens = append(tokens, string(candidate.Token))
	}

	activeToken, hasActive := activeContinuation(node.tokens, activeTokens)

	if !hasActive {
		return tokens
	}

	for _, token := range tokens {
		if token == activeToken {
			return tokens
		}
	}

	return append(tokens, activeToken)
}

/*
activeContinuation reports the token the live sequence takes out of this node,
when this node lies on the live sequence at all.
*/
func activeContinuation(
	nodeTokens []string, activeTokens []string,
) (string, bool) {
	if len(nodeTokens) >= len(activeTokens) {
		return "", false
	}

	for index, token := range nodeTokens {
		if activeTokens[index] != token {
			return "", false
		}
	}

	return activeTokens[len(nodeTokens)], true
}

func (solver *Solver) sequenceBytes(tokens []string) []byte {
	return []byte(strings.Join(tokens, "_"))
}

func (solver *Solver) decodeCategoryPath(path []byte) string {
	tokens := strings.Split(string(path), "_")

	for index := range tokens {
		tokens[index] = solver.decodeCategoryToken(tokens[index])
	}

	return strings.Join(tokens, " → ")
}

/*
formatLookaheadPredictions extracts future token paths from beam search results.
*/
func (solver *Solver) formatLookaheadPredictions(
	paths []dmt.BeamPath, currentPrefix []byte,
) map[string]float64 {
	if len(paths) == 0 {
		return nil
	}

	predictions := make(map[string]float64, len(paths))

	for _, path := range paths {
		// Strip active prefix to show only future projected hops
		futureSuffix := bytes.TrimPrefix(path.Sequence, currentPrefix)
		futureSuffix = bytes.TrimPrefix(futureSuffix, []byte("_"))

		if len(futureSuffix) == 0 {
			continue
		}

		if !utf8.Valid(futureSuffix) {
			continue
		}

		predictions[solver.decodeCategoryPath(futureSuffix)] = math.Exp(path.Score)
	}

	return predictions
}

/*
Reset clears active sequence buffers for all symbols.
*/
func (solver *Solver) Reset() {
	solver.states.Range(func(key any, value any) bool {
		solver.states.Delete(key)
		return true
	})
}

/*
Close cleans up the solver.
*/
func (solver *Solver) Close() error {
	solver.cancel()
	return nil
}
