package cognition

import (
	"bytes"
	"math"
	"strings"
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

	// Reusable zero-allocation scratch buffers
	classScratch dmt.ClassificationScratch
	beamScratch  dmt.BeamSearchScratch

	// Consolidation products, refreshed on the REM schedule rather than per
	// tick, because they read weights only consolidation rewrites.
	dreams  []string
	symbols []types.CognitionSymbol

	// spawned records the last self-named regime per symbol, so a reading can
	// report that its basin was invented rather than taught.
	spawned map[string]string
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
		maxSeqLen:      6,   // Max 6 category transitions per sequence window
		surprisalLimit: 3.5, // > 3.5 bits surprisal (P < 8.8%) indicates a regime break
		ui:             ui,
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
	if !thesis.Readiness.Categories {
		return nil
	}

	solver.tickCounter++
	nowUnix := uint64(thesis.At.UnixNano())

	// 1. Process active categories per symbol
	thesis.Categories.Range(func(key, value interface{}) bool {
		symbol := key.(string)
		categories := value.([]types.Category)

		if len(categories) == 0 {
			return true
		}

		// Select the dominant category for this symbol on this tick
		dominantCategory := solver.selectDominantCategory(categories)

		if dominantCategory == types.CategoryTypeNone {
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
		const beamWidth = 3
		const maxHops = 2

		beamPaths := solver.tree.ExecuteBeamSearch(
			activeSequenceBytes,
			beamWidth,
			maxHops,
			&solver.beamScratch,
		)

		// 6. Measure Branch Ambiguity (Shannon Entropy)
		ambiguity := solver.tree.MeasureBranchAmbiguity(activeSequenceBytes)

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
				Score:    path.Score,
			})
		}

		branches := []types.CognitionBranch{{
			ID:          0,
			ParentID:    -1,
			Token:       "•",
			Probability: 1,
		}}
		prefixTokens := make([]string, 0, len(activeTokens))

		for index, token := range activeTokens {
			prefixTokens = append(prefixTokens, token)
			prefixBytes := solver.sequenceBytes(prefixTokens)
			prefix := solver.decodeCategoryPath(prefixBytes)
			weight := solver.tree.GetSensoryWeight(prefixBytes)
			branches = append(branches, types.CognitionBranch{
				ID:          index + 1,
				ParentID:    index,
				Token:       solver.decodeCategoryToken(token),
				Prefix:      prefix,
				Depth:       index + 1,
				Probability: weight.Probability,
				Count:       weight.Count,
			})
		}

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
			Ready:            winner != "",
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
			BeamWidth:        beamWidth,
			MaxHops:          maxHops,
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
		}

		thesis.Cognition.Store(symbol, cognition)
		return true
	})

	thesis.Stamp(types.SourceCognition)

	solver.publish(thesis)

	// 10. Periodic REM Sleep Consolidation (every 128 ticks)
	if solver.tickCounter%128 == 0 && nowUnix > 60e9 {
		startWindow := nowUnix - 60e9 // 1 minute window
		solver.consolidate(startWindow, nowUnix)
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
	dreams := solver.tree.ExecuteREMSleepWithDreaming(
		startWindow,
		nowUnix,
		dreamTemperature,
		dreamMaxTokens,
		&solver.classScratch,
		dmt.SelectStochasticToken,
	)

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

	thesis.Cognition.Range(func(key, value any) bool {
		symbol, ok := key.(string)
		if !ok || value == nil {
			return true
		}

		cognition, ok := value.(types.Cognition)
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
