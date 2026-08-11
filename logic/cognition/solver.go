package cognition

import (
	"bytes"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/audit"
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

/*
Solver uses dmt.Tree to learn, score, and forecast market category transition sequences,
classify macro regimes via attractor basins, and predict future category paths using beam search.
*/
type Solver struct {
	recorder       *audit.Recorder
	tree           *dmt.Tree
	sequences      map[string][]string // Active category token buffer per symbol
	maxSeqLen      int
	surprisalLimit float64
	tickCounter    uint64
	ui             chan []byte

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

	// Reusable zero-allocation scratch buffers
	classScratch dmt.ClassificationScratch
	beamScratch  dmt.BeamSearchScratch

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

	// spawned records the last self-named regime per symbol, so a reading can
	// report that its basin was invented rather than taught.
	spawned map[string]string

	// branches caches the exported prefix tree per symbol. Walking the trie
	// costs orders more than the rest of a reading, and the walk can only
	// change for a symbol when its own sequence transitions, so a rebuild is
	// tied to transitions rather than run on every tick.
	branches      map[string][]types.CognitionBranch
	branchesStamp map[string]uint64
}

const categoryTokenSeparator = "\x1f"

/*
dreamTemperature is how far consolidation strays from the strongest continuation
when generating from a settled basin. Zero would replay the modal path the model
already holds, which teaches it nothing; the value keeps generation exploratory
while consolidation's own novelty and confidence gates discard what does not
come back crisp.
*/
const dreamTemperature = 0.8

/*
dreamMaxTokens bounds a generated sequence to the same window a lived one gets,
so an invented regime cannot claim more context than an observed one.
*/
const dreamMaxTokens = 6

/*
symbolLimit is how many discriminative category paths are retained for display.
*/
const symbolLimit = 32

/*
NewSolver returns a new cognition solver bound to a radix tree.
*/
func NewSolver(
	tree *dmt.Tree,
	ui chan []byte,
	recorder *audit.Recorder,
	opts ...Option,
) *Solver {
	solver := &Solver{
		recorder:       recorder,
		tree:           tree,
		sequences:      make(map[string][]string),
		spawned:        make(map[string]string),
		branches:       make(map[string][]types.CognitionBranch),
		branchesStamp:  make(map[string]uint64),
		maxSeqLen:      6,   // Max 6 category transitions per sequence window
		surprisalLimit: 3.5, // > 3.5 bits surprisal (P < 8.8%) indicates a regime break
		ui:             ui,
		beamWidth:      3,
		maxHops:        2,
		branchWidth:    4,
		branchDepth:    5,
		maxBranchNodes: 192,
		beamScratch: dmt.BeamSearchScratch{
			CurrentBeams: make([]dmt.BeamPath, 0, 4),
			NextBeams:    make([]dmt.BeamPath, 0, 4),
			LookupBuffer: make([]dmt.LookaheadPrediction, 0, 8),
		},
	}

	for _, opt := range opts {
		opt(solver)
	}

	return solver
}

/*
Update ingests the active Thesis categories, evaluates category transition surprisal,
breaks/continues sequence paths, classifies market regimes, and runs lookahead beam search.
*/
func (solver *Solver) Update(thesis *types.Thesis) error {
	updated := false
	nowUnix := uint64(thesis.At.UnixNano())

	// 1. Process active categories per symbol
	thesis.Symbols.Range(func(key, value interface{}) bool {
		symbol := key.(string)
		symbolState := value.(*types.Symbol)
		stored, found := symbolState.Categories.Load(symbol)

		if !found {
			thesis.Stamp(symbol, types.SourceCognition)
			return true
		}

		categories := stored.([]types.Category)

		if thesis.Stamped(symbol, types.SourceCognition) ||
			!thesis.Stamped(symbol, types.SourceCategory) {
			return true
		}

		if !updated {
			solver.tickCounter++
			updated = true
		}

		if len(categories) == 0 {
			thesis.Stamp(symbol, types.SourceCognition)
			return true
		}

		// Select the dominant category for this symbol on this tick
		dominantCategory := solver.selectDominantCategory(categories)

		if dominantCategory == types.CategoryTypeNone {
			thesis.Stamp(symbol, types.SourceCognition)
			return true
		}

		categoryToken := solver.encodeCategory(dominantCategory)
		activeTokens := solver.sequences[symbol]
		transitioned := len(activeTokens) == 0 ||
			activeTokens[len(activeTokens)-1] != categoryToken

		if transitioned {
			// 2. Evaluate if appending this category causes a Sequence Break
			broken, _ := solver.evalSequenceBreak(activeTokens, categoryToken)

			if broken && len(activeTokens) > 0 {
				// --- SEQUENCE BREAK DETECTED ---
				oldSequenceBytes := solver.sequenceBytes(activeTokens)

				// Commit completed sequence to episodic buffer for REM replay
				_, _ = solver.tree.CommitToEpisodicBuffer(nowUnix, oldSequenceBytes)

				/*
					A completed sequence is learned without being told what it
					was. When nothing the model already knows explains it, the
					model names a regime for it rather than forcing it into the
					least-bad existing basin and corrupting that basin.

					This is the one place cognition can exceed the category
					taxonomy: categories are a fixed vocabulary, but the
					sequences they compose into are not.
				*/
				outcome, experienceErr := solver.tree.ExperienceSequence(
					oldSequenceBytes, &solver.classScratch,
				)

				if experienceErr == nil && outcome.NewConcept {
					solver.spawned[symbol] = string(outcome.Class)
				}

				// Start fresh sequence buffer with new category
				activeTokens = []string{categoryToken}
			} else {
				// --- SEQUENCE CONTINUES ---
				activeTokens = append(activeTokens, categoryToken)
			}

			solver.sequences[symbol] = activeTokens

			// 3. Train only the first observation and category transitions.
			solver.tree.TrainSensorySequence(solver.sequenceBytes(activeTokens))
		}

		activeSequenceBytes := solver.sequenceBytes(activeTokens)
		spawnedClass, spawned := solver.spawned[symbol]

		// 4. Classify macro market regime / concept attractor basin
		classResult := solver.tree.Classify(activeSequenceBytes, &solver.classScratch)

		// 5. Lookahead Beam Search: Predict next 2–3 likely category hops
		beamPaths := solver.tree.ExecuteBeamSearch(
			activeSequenceBytes,
			solver.beamWidth,
			solver.maxHops,
			&solver.beamScratch,
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

		// 7. Format Lookahead Predictions for Thesis
		predictions := solver.formatLookaheadPredictions(
			beamPaths, activeSequenceBytes,
		)

		classes := make([]types.CognitionClass, 0, len(classResult.Scores))

		for _, score := range classResult.Scores {
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

		branches := solver.cachedPrefixTree(symbol, activeTokens, transitioned)

		contrast := 0.0
		contrastEvidence := 0.0

		if len(classResult.Scores) > 1 {
			contrast = classResult.Scores[0].Value - classResult.Scores[1].Value
			evidence := solver.tree.ComputeBasinContrastiveEvidence(
				classResult.Scores[0].ClassName,
				classResult.Scores[1].ClassName,
				activeSequenceBytes,
			)
			contrastEvidence = evidence.Divergence
		}

		lookaheadScore := 0.0

		if len(beamPaths) > 0 {
			lookaheadScore = beamPaths[0].Score
		}

		winner := string(classResult.Winner)

		// Backoff surprisal scores the sequence against every order of its own
		// context, so an unseen continuation of a known prefix is merely
		// surprising instead of unmeasurable.
		interpolated := solver.tree.InterpolatedSurprisal(activeSequenceBytes)
		averageSurprisal := 0.0

		for _, item := range interpolated {
			averageSurprisal += item.Surprisal
		}

		if len(interpolated) > 0 {
			averageSurprisal /= float64(len(interpolated))
		}

		cognition := types.Cognition{
			Source:           "cognition",
			Symbol:           symbol,
			At:               thesis.At,
			Sequence:         solver.decodeCategoryPath(activeSequenceBytes),
			RegimePrefix:     winner,
			Winner:           winner,
			WinnerClass:      winner,
			Confidence:       classResult.Highest,
			ClassConfidence:  classResult.Highest,
			Contrast:         contrast,
			ContrastEvidence: contrastEvidence,
			EntropyBits:      ambiguity.EntropyBits,
			EntropyThreshold: ambiguity.Threshold,
			Ambiguous:        ambiguity.Ambiguous,
			Cohort:           solver.tree.GetSensoryWeight(activeSequenceBytes).Count,
			LookaheadScore:   lookaheadScore,
			LookaheadPaths:   len(beamPaths),
			BeamWidth:        solver.beamWidth,
			MaxHops:          solver.maxHops,
			NodeCount:        len(branches),
			Predictions:      predictions,
			Branches:         branches,
			Beams:            beams,
			Classes:          classes,

			InterpolatedSurprisal: averageSurprisal,
			Contributions:         solver.contributions(activeSequenceBytes),
			Lexical:               solver.lexical(activeTokens),
			Symbols:               solver.symbols,
			Dreams:                solver.dreams,
			NewConcept:            spawned,
			SpawnedClass:          spawnedClass,

			REMFrom:          solver.remFrom,
			REMThrough:       solver.remThrough,
			REMReplays:       int(solver.remOutcome.ReplayedObservations),
			REMDecayFactor:   solver.remOutcome.DecayFactor,
			REMInhibitionPct: solver.remOutcome.RetroactiveInhibitionPct,
			// A pass runs synchronously inline on the 128-tick schedule
			// below, so "consolidating" is true only for the reading
			// published on the very tick that triggered it — every other
			// tick reports the awake state between passes.
			REMConsolidating: solver.tickCounter%128 == 0 && nowUnix > 60e9,
		}

		symbolState.Cognition.Store(symbol, cognition)
		thesis.Stamp(symbol, types.SourceCognition)
		return true
	})

	if updated {
		solver.publish(thesis)

		// 10. Periodic REM Sleep Consolidation (every 128 ticks)
		if solver.tickCounter%128 == 0 && nowUnix > 60e9 {
			startWindow := nowUnix - 60e9 // 1 minute window
			solver.consolidate(startWindow, nowUnix)
		}
	}

	return nil
}

/*
consolidate replays the episodic window and then dreams from each settled basin,
recording what the model invented for itself.

Why:

	Replay can only reinforce what was observed. Generating from a basin and
	keeping what classifies back to it crisply is how the model fills in the
	continuations its own statistics imply but that never happened to occur.

	Symbol extraction runs on the same schedule because it reads the basin
	weights consolidation has just rewritten. Extracting before the pass would
	rank paths on evidence that is about to change.
*/
func (solver *Solver) consolidate(
	startWindow, nowUnix uint64,
) {
	dreams, outcome := solver.tree.ExecuteREMSleepWithDreaming(
		startWindow,
		nowUnix,
		dreamTemperature,
		dreamMaxTokens,
		&solver.classScratch,
		dmt.SelectStochasticToken,
	)

	solver.remOutcome = outcome
	solver.remFrom = time.Unix(0, int64(startWindow))
	solver.remThrough = time.Unix(0, int64(nowUnix))

	solver.dreams = solver.dreams[:0]

	for _, dream := range dreams {
		solver.dreams = append(
			solver.dreams,
			solver.decodeCategoryPath(dream.Sequence),
		)
	}

	solver.symbols = solver.symbols[:0]

	for _, symbol := range solver.tree.ExtractDiscriminativeSymbols(symbolLimit) {
		solver.symbols = append(solver.symbols, types.CognitionSymbol{
			Symbol: solver.decodeCategoryPath(symbol.Symbol),
			Class:  solver.decodeCategoryToken(string(symbol.Class)),
			Score:  symbol.Score,
			Purity: symbol.Purity,
		})
	}
}

/*
contributions reports how much more evidence each transition gave the winning
basin than the runner-up, which is what turns a verdict into an explanation.
*/
func (solver *Solver) contributions(
	sequenceBytes []byte,
) []types.CognitionContribution {
	raw := solver.tree.ContrastiveTokenContributions(sequenceBytes)

	if len(raw) == 0 {
		return nil
	}

	out := make([]types.CognitionContribution, 0, len(raw))

	for _, contribution := range raw {
		out = append(out, types.CognitionContribution{
			Token: solver.decodeCategoryToken(string(contribution.Token)),
			Bits:  contribution.Bits,
		})
	}

	return out
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

	surprisalItems := solver.tree.GetSurprisal(candidateBytes)

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
	symbol string, activeTokens []string, transitioned bool,
) []types.CognitionBranch {
	cached, found := solver.branches[symbol]
	stale := solver.tickCounter-solver.branchesStamp[symbol] >= branchRefreshTicks

	if found && !transitioned && !stale {
		return cached
	}

	branches := solver.prefixTreeBranches(activeTokens)
	solver.branches[symbol] = branches
	solver.branchesStamp[symbol] = solver.tickCounter

	return branches
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
publish emits one cognition wire frame per symbol observed on this tick.
*/
func (solver *Solver) publish(thesis *types.Thesis) {
	if solver.ui == nil || thesis == nil {
		return
	}

	/*
		Rows are keyed by symbol rather than listed, because the display reads
		one symbol at a time and a position in a list says nothing about which
		symbol it describes once the set of symbols changes between ticks.
	*/
	rows := datura.NewMap()

	thesis.Symbols.Range(func(key, value any) bool {
		symbol, ok := key.(string)
		symbolState, stateOK := value.(*types.Symbol)

		if !ok || !stateOK || symbolState == nil {
			return true
		}

		stored, found := symbolState.Cognition.Load(symbol)

		if !found {
			return true
		}

		cognition, ok := stored.(types.Cognition)
		if !ok {
			return true
		}

		cognition.At = thesis.At
		rows[symbol] = cognition
		return true
	})

	if len(rows) > 0 {
		select {
		case solver.ui <- datura.NewMap("cognition", rows).MarshalAndFree():
		default:
		}
	}
}

/*
Reset clears active sequence buffers for all symbols.
*/
func (solver *Solver) Reset() {
	solver.sequences = make(map[string][]string)
}

/*
Close cleans up the solver.
*/
func (solver *Solver) Close() error {
	solver.sequences = nil
	return nil
}
