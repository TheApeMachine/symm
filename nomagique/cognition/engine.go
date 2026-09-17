/*
Package cognition is the associative memory primitive: one engine that
observes context -> class associations into an immutable radix trie of packed
weights, classifies contexts against it with Dirichlet-smoothed backoff, and
measures the information content of what it sees.

Everything is a streaming Primitive over an unsafe.Pointer wire. Payloads are
plain data types with exported fields only and no methods. The engine owns
the trie, its clock, its decay and its class census; observe, evaluate,
snapshot, restore, census and root are intents of one command struct because
they operate on that one state. The packed weight record layout and the store
key layout each have one further public owner (Weight and Key) for callers
that read what the engine has written.
*/
package cognition

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"fmt"
	"io"
	"iter"
	"math"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unsafe"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/probability"
)

type Action string

const (
	ActionEnter Action = "enter"
	ActionExit  Action = "exit"
	ActionWait  Action = "wait"
)

func LegalActions(holding bool) []Action {
	if !holding {
		return []Action{ActionEnter, ActionWait}
	}

	return []Action{ActionExit, ActionWait}
}

const (
	maxSensoryCandidates = 16 // Sensory transition hypothesis space
	maxCandidates        = 16 // Initial candidate scratch capacity
)

/*
Association is one observed precursor and the class that followed it.
Without a grade it observes a positive association. A graded association
carries a dimensionless reinforcement amount: positive strengthens, negative
inhibits, and zero only observes sensory context. It is not stored in a
separate reward model.
*/
type Association struct {
	Context  []byte
	Class    []byte
	Feedback float64
	Graded   bool
}

/*
Question is the context being asked about.
*/
type Question struct {
	Context []byte
	// Exact prevents token backoff across structured application key boundaries.
	Exact bool
}

/*
Snapshot asks the engine to serialize its current model.
*/
type Snapshot struct{}

/*
Census asks for the per-class observation census.
*/
type Census struct{}

/*
Root asks for the current immutable trie.
*/
type Root struct{}

/*
Command discriminates one engine intent. Exactly one field is set; any other
shape is a failure recorded in Error and ends the stream.
*/
type Command struct {
	Observe  *Association
	Evaluate *Question
	Snapshot *Snapshot
	Restore  []byte
	Census   *Census
	Root     *Root
}

/*
Result is one command's answer: the reading an evaluation produced, the trie
an observation published or a root command read, the serialized model, or the
class census.
*/
type Result struct {
	Evaluation Evaluation
	Tree       *iradix.Tree[[]byte]
	Model      []byte
	Classes    map[string]int32
}

/*
Engine is the cognitive memory Primitive. It owns the immutable radix trie,
the observation clock it decays against, and the class census. Concurrent
observes publish through compare-and-swap; evaluations read immutable roots.
*/
type Engine struct {
	*core.PrimitiveError

	cfg         Config
	root        atomic.Pointer[iradix.Tree[[]byte]]
	stepCounter atomic.Uint64
	decayFactor float64
	classCounts sync.Map
	remReplays  atomic.Uint64
}

/*
NewEngine instantiates the cognitive engine Primitive. Unset bounds in the
configuration fold to the declared defaults; the normalized values are
computed here and owned by the engine.
*/
func NewEngine(cfg Config) *Engine {
	engine := &Engine{PrimitiveError: core.NewPrimitiveError(), cfg: cfg.normalised(),
		decayFactor: cfg.normalised().decayFactor(),
	}

	engine.root.Store(iradix.New[[]byte]())
	return engine
}

/* Next executes typed Command pointers and yields the command result. */
func (engine *Engine) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			command := (*Command)(arriving)
			result, err := engine.execute(command)

			if err != nil {
				engine.Error(err)
				return
			}

			if command.Evaluate != nil {
				if !yield(unsafe.Pointer(&result.Evaluation)) {
					return
				}
				continue
			}

			if !yield(unsafe.Pointer(&result)) {
				return
			}
		}
	}
}

/*
execute dispatches one command to its intent and returns its result.
*/
func (engine *Engine) execute(command *Command) (Result, error) {
	intents := 0

	if command.Observe != nil {
		intents++
	}

	if command.Evaluate != nil {
		intents++
	}

	if command.Snapshot != nil {
		intents++
	}

	if command.Restore != nil {
		intents++
	}

	if command.Census != nil {
		intents++
	}

	if command.Root != nil {
		intents++
	}

	if intents != 1 {
		return Result{}, fmt.Errorf(
			"%w: cognition: engine command must set exactly one intent",
			core.ErrShape,
		)
	}

	if command.Observe != nil {
		return engine.observe(*command.Observe)
	}

	if command.Evaluate != nil {
		return engine.evaluate(command.Evaluate.Context, command.Evaluate.Exact)
	}

	if command.Snapshot != nil {
		return engine.snapshot()
	}

	if command.Restore != nil {
		return engine.restore(command.Restore)
	}

	if command.Census != nil {
		return Result{Classes: engine.census()}, nil
	}

	return Result{Tree: engine.root.Load()}, nil
}

// Evaluate classifies a context sequence against the radix trie.
func (engine *Engine) Evaluate(context []byte) (Result, error) {
	return engine.evaluate(context)
}

// Observe records a context -> class association into the radix trie.
func (engine *Engine) Observe(assoc Association) (Result, error) {
	return engine.observe(assoc)
}

// Train ingests a sequence of sensory tokens associated with a target class,
// decomposing it into suffix n-grams up to MaxBackoffOrder with surprisal-modulated plasticity.
func (engine *Engine) Train(sequence []byte, class []byte, feedback float64) (Result, error) {
	if len(sequence) == 0 {
		return Result{}, errnie.Error(errnie.Err(errnie.Validation, "cognition: sequence is required for training", nil))
	}

	if len(class) == 0 {
		return Result{}, errnie.Error(errnie.Err(errnie.Validation, "cognition: class is required for training", nil))
	}

	evalResult, evalErr := engine.evaluate(sequence)

	if evalErr != nil {
		return Result{}, evalErr
	}

	plasticity := math.Min(1.0, 0.1+(evalResult.Evaluation.Surprisal/4.0))
	effectiveFeedback := feedback * plasticity
	var lastResult Result
	var err error

	if len(sequence) >= 12 {
		offset := 0
		var frameOffsets []int

		for offset < len(sequence) {
			if offset+4 > len(sequence) {
				frameOffsets = nil
				break
			}

			count := binary.BigEndian.Uint32(sequence[offset : offset+4])

			if count == 0 {
				frameOffsets = nil
				break
			}

			frameSize := 4 + int(count)*8

			if offset+frameSize > len(sequence) {
				frameOffsets = nil
				break
			}

			frameOffsets = append(frameOffsets, offset)
			offset += frameSize
		}

		if offset == len(sequence) && len(frameOffsets) > 0 {
			maxOrder := engine.cfg.MaxBackoffOrder

			for startIdx := 0; startIdx < len(frameOffsets); startIdx++ {
				limitIdx := min(len(frameOffsets), startIdx+maxOrder)

				for endIdx := startIdx + 1; endIdx <= limitIdx; endIdx++ {
					var subContext []byte

					if endIdx < len(frameOffsets) {
						subContext = sequence[frameOffsets[startIdx]:frameOffsets[endIdx]]
					}

					if endIdx >= len(frameOffsets) {
						subContext = sequence[frameOffsets[startIdx]:]
					}

					lastResult, err = engine.observe(Association{
						Context:  subContext,
						Class:    class,
						Feedback: effectiveFeedback,
						Graded:   true,
					})

					if err != nil {
						return Result{}, err
					}
				}
			}

			return lastResult, nil
		}
	}

	if len(sequence)%8 == 0 && len(sequence) >= 8 {
		tokenCount := len(sequence) / 8
		maxOrder := engine.cfg.MaxBackoffOrder

		for startIdx := 0; startIdx < tokenCount; startIdx++ {
			limitIdx := min(tokenCount, startIdx+maxOrder)

			for endIdx := startIdx + 1; endIdx <= limitIdx; endIdx++ {
				subContext := sequence[startIdx*8 : endIdx*8]
				lastResult, err = engine.observe(Association{
					Context:  subContext,
					Class:    class,
					Feedback: effectiveFeedback,
					Graded:   true,
				})

				if err != nil {
					return Result{}, err
				}
			}
		}

		return lastResult, nil
	}

	delim := byte(0)
	hasDelim := false

	if bytes.IndexByte(sequence, 0) >= 0 {
		delim = 0
		hasDelim = true
	}

	if !hasDelim && bytes.IndexByte(sequence, '/') >= 0 {
		delim = '/'
		hasDelim = true
	}

	if !hasDelim && bytes.IndexByte(sequence, '_') >= 0 {
		delim = '_'
		hasDelim = true
	}

	if hasDelim {
		var tokenBounds [][]int
		startOffset := 0

		for currentOffset, charByte := range sequence {
			if charByte == delim {
				if currentOffset > startOffset {
					tokenBounds = append(tokenBounds, []int{startOffset, currentOffset})
				}

				startOffset = currentOffset + 1
			}
		}

		if len(sequence) > startOffset {
			tokenBounds = append(tokenBounds, []int{startOffset, len(sequence)})
		}

		if len(tokenBounds) > 0 {
			maxOrder := engine.cfg.MaxBackoffOrder

			for startIdx := 0; startIdx < len(tokenBounds); startIdx++ {
				limitIdx := min(len(tokenBounds), startIdx+maxOrder)

				for endIdx := startIdx + 1; endIdx <= limitIdx; endIdx++ {
					subContext := sequence[tokenBounds[startIdx][0]:tokenBounds[endIdx-1][1]]
					lastResult, err = engine.observe(Association{
						Context:  subContext,
						Class:    class,
						Feedback: effectiveFeedback,
						Graded:   true,
					})

					if err != nil {
						return Result{}, err
					}
				}
			}

			return lastResult, nil
		}
	}

	lastResult, err = engine.observe(Association{
		Context:  sequence,
		Class:    class,
		Feedback: effectiveFeedback,
		Graded:   true,
	})

	if err != nil {
		return Result{}, err
	}

	_, suffixes := backoffCandidates(sequence, engine.cfg.MaxBackoffOrder)

	for _, suffix := range suffixes {
		lastResult, err = engine.observe(Association{
			Context:  suffix,
			Class:    class,
			Feedback: effectiveFeedback,
			Graded:   true,
		})

		if err != nil {
			return Result{}, err
		}
	}

	return lastResult, nil
}

// Prune walks the radix trie and removes records whose effective count
// has dropped below minEffectiveCount.
func (engine *Engine) Prune(minEffectiveCount float64) int {
	if minEffectiveCount <= 0 {
		minEffectiveCount = 0.05
	}

	prunedTotal := 0

	for {
		oldRoot := engine.root.Load()
		currentStep := engine.stepCounter.Load()
		txn := oldRoot.Txn()

		var keysToDelete [][]byte
		iterator := oldRoot.Root().Iterator()

		for keyBytes, valBytes, ok := iterator.Next(); ok; keyBytes, valBytes, ok = iterator.Next() {
			if len(valBytes) != WeightSize {
				continue
			}

			weight := decodeWeight(valBytes).effective(currentStep, engine.decayFactor)
			effectiveMass := float64(weight.Count) * weight.Probability

			if effectiveMass < minEffectiveCount {
				keysToDelete = append(keysToDelete, bytes.Clone(keyBytes))
			}
		}

		if len(keysToDelete) == 0 {
			return 0
		}

		for _, keyBytes := range keysToDelete {
			txn.Delete(keyBytes)
		}

		newRoot := txn.Commit()

		if engine.root.CompareAndSwap(oldRoot, newRoot) {
			prunedTotal = len(keysToDelete)
			break
		}
	}

	return prunedTotal
}

// Dream generates a candidate continuation sequence via temperature-guided lookahead.
func (engine *Engine) Dream(temperature float64, maxLength int) string {
	if maxLength <= 0 {
		maxLength = 64
	}

	root := engine.root.Load()
	currentSequence := ""

	for hop := 0; hop < maxLength; hop++ {
		searchPrefix := makeSensoryKey([]byte(currentSequence))
		iterator := root.Root().Iterator()
		iterator.SeekPrefix(searchPrefix)

		var candidates []string
		var probabilities []float64

		for keyBytes, valBytes, ok := iterator.Next(); ok; keyBytes, valBytes, ok = iterator.Next() {
			if !bytes.HasPrefix(keyBytes, searchPrefix) {
				break
			}

			seqSuffix := string(keyBytes[len("s/"):])

			if len(seqSuffix) <= len(currentSequence) {
				continue
			}

			weight := decodeWeight(valBytes)
			candidates = append(candidates, seqSuffix)
			probabilities = append(probabilities, math.Max(weight.Probability, 1e-4))
		}

		if len(candidates) == 0 {
			break
		}

		if temperature <= 0 {
			bestIdx := 0
			bestProb := probabilities[0]

			for candIdx := 1; candIdx < len(probabilities); candIdx++ {
				if probabilities[candIdx] > bestProb {
					bestProb = probabilities[candIdx]
					bestIdx = candIdx
				}
			}

			currentSequence = candidates[bestIdx]
			continue
		}

		totalScaled := 0.0
		scaledWeights := make([]float64, len(probabilities))

		for candIdx, probVal := range probabilities {
			scaled := math.Pow(probVal, 1.0/temperature)
			scaledWeights[candIdx] = scaled
			totalScaled += scaled
		}

		sampleVal := rand.Float64() * totalScaled
		runningSum := 0.0
		selectedCandidate := candidates[len(candidates)-1]

		for candIdx, scaled := range scaledWeights {
			runningSum += scaled

			if sampleVal <= runningSum {
				selectedCandidate = candidates[candIdx]
				break
			}
		}

		currentSequence = selectedCandidate
	}

	return currentSequence
}

// Consolidate executes an offline REM sleep memory consolidation cycle.
func (engine *Engine) Consolidate(temperature float64) (string, string, float64, bool, error) {
	classes := engine.census()

	if len(classes) == 0 {
		return "", "", 0, false, nil
	}

	targetClass := ""
	minCount := int32(math.MaxInt32)

	for clsName, countVal := range classes {
		if countVal < minCount {
			minCount = countVal
			targetClass = clsName
		}
	}

	if targetClass == "" {
		for clsName := range classes {
			targetClass = clsName
			break
		}
	}

	dream := engine.Dream(temperature, 64)

	if len(dream) == 0 {
		return "", targetClass, 0, false, nil
	}

	evalResult, evalErr := engine.evaluate([]byte(dream))

	if evalErr != nil {
		return dream, targetClass, 0, false, evalErr
	}

	confidence := evalResult.Evaluation.Confidence
	novel := false

	if evalResult.Evaluation.WinnerClass == targetClass && confidence >= 0.80 {
		basinKey := makeBasinKey([]byte(targetClass), []byte(dream))
		root := engine.root.Load()
		_, exists := root.Get(basinKey)
		novel = !exists

		if novel {
			_, trainErr := engine.Train([]byte(dream), []byte(targetClass), 1.0)

			if trainErr != nil {
				return dream, targetClass, confidence, novel, trainErr
			}

			engine.remReplays.Add(1)
		}
	}

	return dream, targetClass, confidence, novel, nil
}

// ExtractSymbols identifies distinctive sequence motifs across attractor basins.
func (engine *Engine) ExtractSymbols() []Symbol {
	root := engine.root.Load()
	currentStep := engine.stepCounter.Load()

	contexts := make(map[string]map[string]float64)
	contextTotals := make(map[string]float64)

	iterator := root.Root().Iterator()
	iterator.SeekPrefix([]byte("b/"))

	for keyBytes, valBytes, ok := iterator.Next(); ok; keyBytes, valBytes, ok = iterator.Next() {
		if !bytes.HasPrefix(keyBytes, []byte("b/")) {
			break
		}

		classBytes, contextBytes, valid := parseBasinKey(keyBytes)

		if !valid || len(contextBytes) == 0 || len(classBytes) == 0 {
			continue
		}

		weight := decodeWeight(valBytes).effective(currentStep, engine.decayFactor)
		effectiveCount := float64(weight.Count) * weight.Probability

		if effectiveCount <= 0 {
			continue
		}

		ctxStr := string(contextBytes)
		clsStr := string(classBytes)

		if contexts[ctxStr] == nil {
			contexts[ctxStr] = make(map[string]float64)
		}

		contexts[ctxStr][clsStr] += effectiveCount
		contextTotals[ctxStr] += effectiveCount
	}

	var results []Symbol

	for ctxStr, classCounts := range contexts {
		total := contextTotals[ctxStr]

		if total < 2.0 {
			continue
		}

		for clsStr, countVal := range classCounts {
			purity := countVal / total
			score := purity * math.Log1p(total)

			if score > 1.0 {
				results = append(results, Symbol{
					Symbol: ctxStr,
					Class:  clsStr,
					Score:  score,
					Purity: purity,
				})
			}
		}
	}

	sort.Slice(results, func(leftIdx, rightIdx int) bool {
		return results[leftIdx].Score > results[rightIdx].Score
	})

	if len(results) > 50 {
		results = results[:50]
	}

	return results
}

// ExportTree constructs the prefix branches, lookahead beams, and class readouts for visualizers.
func (engine *Engine) ExportTree(activeContext []byte, maxBranches int) TreeExport {
	if maxBranches <= 0 {
		maxBranches = 128
	}

	root := engine.root.Load()
	currentStep := engine.stepCounter.Load()
	var branches []Branch
	nodeMap := make(map[string]int)

	branches = append(branches, Branch{
		ID:          0,
		ParentID:    -1,
		Token:       "root",
		Prefix:      "",
		Key:         "",
		Depth:       0,
		Probability: 1.0,
		Count:       currentStep,
	})
	nodeMap[""] = 0

	iterator := root.Root().Iterator()
	iterator.SeekPrefix([]byte("s/"))

	for keyBytes, valBytes, ok := iterator.Next(); ok; keyBytes, valBytes, ok = iterator.Next() {
		if !bytes.HasPrefix(keyBytes, []byte("s/")) {
			break
		}

		if len(branches) >= maxBranches {
			break
		}

		seqBytes := keyBytes[len("s/"):]

		if len(seqBytes) == 0 {
			continue
		}

		seqStr := string(seqBytes)
		state := decodeWeight(valBytes).effective(currentStep, engine.decayFactor)
		parentID := 0
		depth := 1
		token := seqStr

		if len(seqBytes)%8 == 0 && len(seqBytes) > 8 {
			parentSeq := string(seqBytes[:len(seqBytes)-8])

			if pid, exists := nodeMap[parentSeq]; exists {
				parentID = pid
				depth = branches[pid].Depth + 1
				token = fmt.Sprintf("%x", binary.BigEndian.Uint64(seqBytes[len(seqBytes)-8:]))
			}
		}

		if strings.Contains(seqStr, "_") {
			lastSep := strings.LastIndex(seqStr, "_")

			if lastSep > 0 {
				parentSeq := seqStr[:lastSep]

				if pid, exists := nodeMap[parentSeq]; exists {
					parentID = pid
					depth = branches[pid].Depth + 1
					token = seqStr[lastSep+1:]
				}
			}
		}

		nodeID := len(branches)
		nodeMap[seqStr] = nodeID

		branches = append(branches, Branch{
			ID:          nodeID,
			ParentID:    parentID,
			Token:       token,
			Prefix:      seqStr,
			Key:         seqStr,
			Depth:       depth,
			Probability: state.Probability,
			Count:       state.Count,
		})
	}

	beams := beamSearch(root, activeContext, engine.cfg.BeamWidth, engine.cfg.MaxHops)
	evalResult, _ := engine.evaluate(activeContext)

	return TreeExport{
		Branches:  branches,
		Beams:     beams,
		Classes:   evalResult.Evaluation.Candidates,
		NodeCount: root.Len(),
	}
}

// REMReplays returns the total number of REM consolidations performed.
func (engine *Engine) REMReplays() int {
	return int(engine.remReplays.Load())
}

/*
observe registers context -> class in the existing packed basin and publishes
the next immutable root through compare-and-swap. Keys are namespaced:
b/<context>/<class> for basins; s/<context> for sensory transitions.
*/
func (engine *Engine) observe(assoc Association) (Result, error) {
	if len(assoc.Context) == 0 {
		return Result{}, fmt.Errorf(
			"%w: cognition: observation requires a context",
			core.ErrDomain,
		)
	}

	basinKey := makeBasinKey(assoc.Class, assoc.Context)
	sensoryKey := makeSensoryKey(assoc.Context)

	var valBuf [WeightSize]byte

	for {
		oldRoot := engine.root.Load()
		step := engine.stepCounter.Add(1)
		txn := oldRoot.Txn()

		isNew := false

		// A zero grade observes the context without reinforcing an action.
		if len(assoc.Class) > 0 && (!assoc.Graded || assoc.Feedback != 0) {
			weight := PackedWeight{Probability: 1, WriteStep: step}

			if assoc.Graded {
				weight.Probability = 0.5 // Neutral between reinforcement and inhibition.
			}

			existing, found := oldRoot.Get(basinKey)

			if found {
				weight = decodeWeight(existing).effective(step, engine.decayFactor)
			}

			isNew = !found
			weight.Count++
			weight.WriteStep = step
			reinforce(&weight, assoc.Feedback, assoc.Graded)
			encodeWeight(valBuf[:], weight)
			txn.Insert(basinKey, bytes.Clone(valBuf[:]))
		}

		// Sensory suffix transition.
		sState := PackedWeight{Count: 1, Probability: 1.0, WriteStep: step}

		if existing, found := oldRoot.Get(sensoryKey); found {
			prior := decodeWeight(existing).effective(step, engine.decayFactor)
			sState.Count = prior.Count + 1
			sState.Probability = prior.Probability + (1.0-prior.Probability)/(float64(sState.Count)+1.0)
		}

		encodeWeight(valBuf[:], sState)
		txn.Insert(sensoryKey, bytes.Clone(valBuf[:]))

		newRoot := txn.Commit()

		if engine.root.CompareAndSwap(oldRoot, newRoot) {
			if isNew {
				engine.incrementClass(string(assoc.Class))
			}

			return Result{Tree: newRoot}, nil
		}
	}
}

/*
evaluate performs single-pass classification, ambiguity gating, surprisal
calculation, and lookahead.
*/
func (engine *Engine) evaluate(context []byte, exact ...bool) (Result, error) {
	if len(context) == 0 {
		return Result{Evaluation: Evaluation{
			Surprisal: engine.cfg.SurprisalBreakBits,
			IsBreak:   true,
		}}, nil
	}

	root := engine.root.Load()
	step := engine.stepCounter.Load()

	// -------------------------------------------------------------
	// 1. Attractor Basin Softmax & Contrast (Nomagique Probability)
	// -------------------------------------------------------------
	var names [maxCandidates][]byte
	var masses [maxCandidates]float64
	var counts [maxCandidates]uint64
	var orders [maxCandidates]int
	acc := classAccumulator{names: names[:0], masses: masses[:0], counts: counts[:0], orders: orders[:0]}
	// Fast path: direct exact prefix lookup b/<context>/ in O(L) time
	exactPrefix := make([]byte, 2+len(context)+1)
	exactPrefix[0] = 'b'
	exactPrefix[1] = '/'
	copy(exactPrefix[2:], context)
	exactPrefix[2+len(context)] = '/'

	it := root.Root().Iterator()
	it.SeekPrefix(exactPrefix)

	for k, v, ok := it.Next(); ok; k, v, ok = it.Next() {
		if !bytes.HasPrefix(k, exactPrefix) {
			break
		}

		class, _, valid := parseBasinKey(k)

		if !valid {
			continue
		}

		state := decodeWeight(v).effective(step, engine.decayFactor)
		mass := float64(state.Count) * state.Probability
		acc.add(class, mass, state.Count, engine.cfg.MaxBackoffOrder)
	}

	// Fallback: if no exact match, use prefix and suffix backoff via direct SeekPrefix
	if acc.count == 0 && (len(exact) == 0 || !exact[0]) {
		maxSteps := engine.cfg.MaxBackoffOrder

		prefixes, suffixes := backoffCandidates(context, maxSteps)
		var keyBuf [512]byte

		// Check suffixes (temporal Markov order reduction: keeping most recent tokens)
		for _, sub := range suffixes {
			var sensoryKey []byte

			if 2+len(sub) <= len(keyBuf) {
				keyBuf[0] = 's'
				keyBuf[1] = '/'
				copy(keyBuf[2:], sub)
				sensoryKey = keyBuf[:2+len(sub)]
			}

			if sensoryKey == nil {
				sensoryKey = makeSensoryKey(sub)
			}

			if _, exists := root.Get(sensoryKey); exists {
				if searchSubPrefix(root, sub, step, engine.decayFactor, 1, engine.cfg.MaxBackoffOrder, &acc) {
					break
				}
			}
		}

		// Check prefixes if no suffix matched
		if acc.count == 0 {
			order := max(1, engine.cfg.MaxBackoffOrder/2)

			for _, sub := range prefixes {
				if searchSubPrefix(root, sub, step, engine.decayFactor, order, engine.cfg.MaxBackoffOrder, &acc) {
					break
				}
			}
		}
	}

	eval := Evaluation{Context: context, Step: step}

	if acc.count > 0 {
		reading, err := engine.classify(&acc)
		if err != nil {
			return Result{}, err
		}

		eval.Support = reading.support
		eval.WinnerClass = reading.winner
		eval.RunnerUp = reading.runnerUp
		eval.Confidence = reading.confidence
		eval.Contrast = reading.contrast
		eval.Ambiguity = reading.ambiguity
		eval.Candidates = reading.candidates
	}

	// -------------------------------------------------------------
	// 2. Surprisal & Sequence-Break Detection
	// -------------------------------------------------------------
	sensoryKey := makeSensoryKey(context)

	totalSteps := float64(step)
	if totalSteps < 1.0 {
		totalSteps = 1.0
	}

	if raw, found := root.Get(sensoryKey); found {
		state := decodeWeight(raw).effective(step, engine.decayFactor)

		prob := (float64(state.Count) + engine.cfg.DirichletAlpha) / (totalSteps + engine.cfg.DirichletAlpha*float64(maxSensoryCandidates))
		if prob > 0 {
			eval.Surprisal = -math.Log2(prob)
		} else {
			eval.Surprisal = engine.cfg.SurprisalBreakBits
		}
	} else {
		// Unseen transition: surprisal derives from Dirichlet baseline over sensory space
		eval.Surprisal = -math.Log2(engine.cfg.DirichletAlpha / (totalSteps + engine.cfg.DirichletAlpha*float64(maxSensoryCandidates)))
	}

	eval.IsBreak = eval.Surprisal >= engine.cfg.SurprisalBreakBits

	// -------------------------------------------------------------
	// 3. Multi-Hop Lookahead (Beam Search)
	// -------------------------------------------------------------
	eval.Lookahead = beamSearch(root, context, engine.cfg.BeamWidth, engine.cfg.MaxHops)

	return Result{Evaluation: eval}, nil
}

/*
classification is the attractor basin readout of one accumulated candidate set.
*/
type classification struct {
	support    uint64
	winner     string
	runnerUp   string
	confidence float64
	contrast   float64
	ambiguity  float64
	candidates []ClassCandidate
}

/*
classify turns accumulated basin logits into the normalized classification:
softmax densities scaled by backoff order, Dirichlet prior mass for unobserved
candidates, and the canonical Argmax, EvidenceShare and ShannonAmbiguity
reductions for the winner, its share, its contrast and the ambiguity.
*/
func (engine *Engine) classify(acc *classAccumulator) (classification, error) {
	var reading classification

	if acc.count == 0 {
		return reading, nil
	}

	totalMass := 0.0
	for i := 0; i < acc.count; i++ {
		totalMass += acc.masses[i]
	}

	distinctTotal := engine.distinctClasses()
	k := max(acc.count+1, distinctTotal)
	unobservedCount := k - acc.count

	alpha := engine.cfg.DirichletAlpha
	denom := totalMass + float64(k)*alpha

	densities := make([]float64, acc.count)
	for i := 0; i < acc.count; i++ {
		densities[i] = (acc.masses[i] + alpha) / denom
	}

	unseenDensity := float64(unobservedCount) * alpha / denom
	densities = append(densities, unseenDensity)

	winner, hasWinner := argmax(densities)

	if hasWinner {
		if winner.Index < acc.count {
			reading.winner = string(acc.names[winner.Index])
			reading.support = acc.counts[winner.Index]
		}

		confidence, err := evidenceShare(densities, winner.Index)
		if err != nil {
			return reading, err
		}

		reading.confidence = confidence

		// Contrast: log-odds divergence against the runner-up.
		runnerUpIdx := -1
		secondBest := -1.0

		for i := 0; i < len(densities); i++ {
			if i == winner.Index {
				continue
			}

			if densities[i] > secondBest {
				secondBest = densities[i]
				runnerUpIdx = i
			}
		}

		if runnerUpIdx >= 0 {
			if runnerUpIdx < acc.count {
				reading.runnerUp = string(acc.names[runnerUpIdx])
			}

			if runnerUpIdx >= acc.count {
				reading.runnerUp = "prior"
			}

			runnerUpShare, err := evidenceShare(densities, runnerUpIdx)
			if err != nil {
				return reading, err
			}

			if runnerUpShare > 0 && reading.confidence > 0 {
				reading.contrast = math.Log2(reading.confidence / runnerUpShare)
			}
		}
	}

	reading.candidates = make([]ClassCandidate, acc.count)

	for i := 0; i < acc.count; i++ {
		share, err := evidenceShare(densities, i)
		if err != nil {
			return reading, err
		}

		reading.candidates[i] = ClassCandidate{
			Name:        string(acc.names[i]),
			Probability: share,
			Support:     acc.counts[i],
			Order:       acc.orders[i],
		}
	}

	// ShannonAmbiguity rewrites its wire in place, so it reads the densities
	// only after every share has been taken from them.
	ambiguity, err := shannonAmbiguity(densities)
	if err != nil {
		return reading, err
	}

	reading.ambiguity = ambiguity

	return reading, nil
}

/*
census reads how often each class has been observed.
*/
func (engine *Engine) census() map[string]int32 {
	res := make(map[string]int32)
	engine.classCounts.Range(func(key, value any) bool {
		if k, ok := key.(string); ok {
			if cnt, ok := value.(*atomic.Int32); ok {
				res[k] = cnt.Load()
			}
		}

		return true
	})

	return res
}

// Census reads how often each class has been observed.
func (engine *Engine) Census() map[string]int32 {
	return engine.census()
}

// Len returns the number of entries stored in the immutable radix trie.
func (engine *Engine) Len() int {
	root := engine.root.Load()
	if root == nil {
		return 0
	}

	return root.Len()
}

func (engine *Engine) distinctClasses() int {
	count := 0
	engine.classCounts.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

func (engine *Engine) incrementClass(class string) {
	val, _ := engine.classCounts.LoadOrStore(class, &atomic.Int32{})

	if cnt, ok := val.(*atomic.Int32); ok {
		cnt.Add(1)
	}
}

/*
snapshot serializes one immutable trie snapshot, its configuration and clock.
There is no checkpoint model: the values are the same packed weights read by
evaluate. Storage I/O belongs to the caller.
*/
func (engine *Engine) snapshot() (Result, error) {
	root := engine.root.Load()
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)

	for _, value := range []any{"cognition/packed-weight/1", engine.cfg, engine.stepCounter.Load(), root.Len()} {
		if err := encoder.Encode(value); err != nil {
			return Result{}, errnie.Error(err)
		}
	}

	iterator := root.Root().Iterator()

	for key, value, found := iterator.Next(); found; key, value, found = iterator.Next() {
		if err := encoder.Encode(key); err != nil {
			return Result{}, errnie.Error(err)
		}

		if err := encoder.Encode(value); err != nil {
			return Result{}, errnie.Error(err)
		}
	}

	return Result{Model: buffer.Bytes()}, nil
}

/*
restore reads a serialized model into a fresh engine before it is shared.
Invalid or retired formats fail explicitly; neither partial state nor a
replacement empty model is published after a failed read.
*/
func (engine *Engine) restore(encoded []byte) (Result, error) {
	if engine.root.Load().Len() != 0 || engine.stepCounter.Load() != 0 {
		return Result{}, errnie.Error(errnie.Err(errnie.Conflict, "cognition: restore requires a fresh engine", nil))
	}

	decoder := gob.NewDecoder(bytes.NewReader(encoded))
	var format string
	var config Config
	var step uint64
	var count int

	for _, destination := range []any{&format, &config, &step, &count} {
		if err := decoder.Decode(destination); err != nil {
			return Result{}, errnie.Error(errnie.Err(errnie.Validation, "cognition: invalid packed model header", err))
		}
	}

	if format != "cognition/packed-weight/1" || count < 0 {
		return Result{}, errnie.Error(errnie.Err(errnie.Validation, "cognition: unsupported packed model format", nil))
	}

	if config.DirichletAlpha <= 0 || config.MaxBackoffOrder <= 0 || config.SurprisalBreakBits <= 0 {
		return Result{}, errnie.Error(errnie.Err(errnie.Validation, "cognition: invalid model configuration", nil))
	}

	transaction := iradix.New[[]byte]().Txn()

	for range count {
		var key, value []byte

		for _, destination := range []any{&key, &value} {
			if err := decoder.Decode(destination); err != nil {
				return Result{}, errnie.Error(errnie.Err(errnie.Validation, "cognition: incomplete packed model", err))
			}
		}

		if err := engine.validate(key, value, step); err != nil {
			return Result{}, err
		}

		if _, replaced := transaction.Insert(key, value); replaced {
			return Result{}, errnie.Error(errnie.Err(errnie.Validation, "cognition: duplicate packed model key", nil))
		}
	}

	var trailing any

	if err := decoder.Decode(&trailing); err != io.EOF {
		return Result{}, errnie.Error(errnie.Err(errnie.Validation, "cognition: trailing packed model data", err))
	}

	engine.cfg = config
	engine.decayFactor = config.decayFactor()
	engine.stepCounter.Store(step)
	root := transaction.Commit()
	engine.root.Store(root)

	return Result{Tree: root}, nil
}

/*
validate rejects records that are neither packed basins nor sensory
transitions, and weights outside their domain.
*/
func (engine *Engine) validate(key, value []byte, step uint64) error {
	_, _, basin := parseBasinKey(key)
	sensory := bytes.HasPrefix(key, []byte("s/")) && len(key) > len("s/")

	if (!basin && !sensory) || len(value) != WeightSize {
		return errnie.Error(errnie.Err(errnie.Validation, "cognition: invalid packed model record", nil))
	}

	weight := decodeWeight(value)

	if weight.Count == 0 || weight.WriteStep > step || weight.Probability < 0 || weight.Probability > 1 {
		return errnie.Error(errnie.Err(errnie.Validation, "cognition: invalid packed model weight", nil))
	}

	return nil
}

/*
beamSearch explores continuation paths using the underlying radix tree iterator.
*/
func beamSearch(root *iradix.Tree[[]byte], prefix []byte, width, hops int) []LookaheadPath {
	if width <= 0 || hops <= 0 || len(prefix) == 0 {
		return nil
	}

	currentPaths := []LookaheadPath{{Sequence: string(prefix), Score: 0.0}}

	for hop := 0; hop < hops; hop++ {
		var candidates []LookaheadPath

		for _, p := range currentPaths {
			searchPrefix := makeSensoryKey([]byte(p.Sequence))
			it := root.Root().Iterator()
			it.SeekPrefix(searchPrefix)

			for k, v, ok := it.Next(); ok; k, v, ok = it.Next() {
				if !bytes.HasPrefix(k, searchPrefix) {
					break
				}

				seq := k[len("s/"):]

				if len(seq) <= len(p.Sequence) {
					continue
				}

				state := decodeWeight(v)
				prob := math.Max(state.Probability, 1e-4)
				logP := math.Log(prob)

				candidates = append(candidates, LookaheadPath{
					Sequence: string(seq),
					Score:    p.Score + logP,
				})
			}
		}

		if len(candidates) == 0 {
			break
		}

		sort.Slice(candidates, func(i, j int) bool {
			return candidates[i].Score > candidates[j].Score
		})

		if len(candidates) > width {
			candidates = candidates[:width]
		}

		currentPaths = candidates
	}

	return currentPaths
}

/*
classAccumulator is the zero-allocation candidate set one evaluation gathers.
*/
type classAccumulator struct {
	names  [][]byte
	masses []float64
	counts []uint64
	orders []int
	count  int
}

func (classAccumulator *classAccumulator) add(name []byte, mass float64, count uint64, order int) {
	for i := 0; i < classAccumulator.count; i++ {
		if bytes.Equal(classAccumulator.names[i], name) {
			if order > classAccumulator.orders[i] || (order == classAccumulator.orders[i] && mass > classAccumulator.masses[i]) {
				classAccumulator.masses[i] = mass
				classAccumulator.counts[i] = count
				classAccumulator.orders[i] = order
			}

			return
		}
	}

	classAccumulator.names = append(classAccumulator.names, name)
	classAccumulator.masses = append(classAccumulator.masses, mass)
	classAccumulator.counts = append(classAccumulator.counts, count)
	classAccumulator.orders = append(classAccumulator.orders, order)
	classAccumulator.count++
}

/*
searchSubPrefix gathers basin candidates under one backoff prefix.
*/
func searchSubPrefix(
	root *iradix.Tree[[]byte], sub []byte, step uint64, decayFactor float64, order int, maxOrder int, acc *classAccumulator,
) bool {
	if len(sub) == 0 {
		return false
	}

	prefixBuf := make([]byte, 2+len(sub)+1)
	prefixBuf[0] = 'b'
	prefixBuf[1] = '/'
	copy(prefixBuf[2:], sub)
	prefixBuf[2+len(sub)] = '/'

	it := root.Root().Iterator()
	it.SeekPrefix(prefixBuf)
	found := false

	for k, v, ok := it.Next(); ok; k, v, ok = it.Next() {
		if !bytes.HasPrefix(k, prefixBuf) {
			break
		}

		class, _, valid := parseBasinKey(k)

		if !valid {
			continue
		}

		state := decodeWeight(v).effective(step, decayFactor)
		mass := float64(state.Count) * state.Probability
		if maxOrder > 0 {
			mass *= float64(order) / float64(maxOrder)
		}
		acc.add(class, mass, state.Count, order)
		found = true
	}

	return found
}

/*
backoffCandidates derives prefix and suffix backoff sub-contexts: structural
length-framed timesteps first ([count uint32][count * 8B tokens]), then
8-byte token alignment, then delimiter splits, then halves.
*/
func backoffCandidates(context []byte, maxSteps int) (prefixes [][]byte, suffixes [][]byte) {
	if len(context) <= 1 || maxSteps <= 0 {
		return nil, nil
	}

	// 1. Structural length-framed timesteps: each frame is [count uint32 (4B)][count * 8B tokens].
	// Valid framed context starts at 0, repeatedly reads 4 bytes count, and advances 4 + count*8 bytes.
	if len(context) >= 12 {
		offset := 0
		var frameOffsets []int

		for offset < len(context) {
			if offset+4 > len(context) {
				frameOffsets = nil
				break
			}

			count := binary.BigEndian.Uint32(context[offset : offset+4])

			if count == 0 {
				frameOffsets = nil
				break
			}

			frameSize := 4 + int(count)*8

			if offset+frameSize > len(context) {
				frameOffsets = nil
				break
			}

			frameOffsets = append(frameOffsets, offset)
			offset += frameSize
		}

		if offset == len(context) && len(frameOffsets) > 1 {
			for step := 1; step < len(frameOffsets); step++ {
				suffixes = append(suffixes, context[frameOffsets[step]:])
			}

			prefSteps := min(len(frameOffsets)-1, maxSteps)

			for step := 1; step <= prefSteps; step++ {
				endIdx := frameOffsets[len(frameOffsets)-step]
				prefixes = append(prefixes, context[:endIdx])
			}

			return prefixes, suffixes
		}
	}

	if len(context)%8 == 0 && len(context) > 8 {
		tokens := len(context) / 8
		steps := min(tokens-1, maxSteps)

		for s := 1; s <= steps; s++ {
			suffixes = append(suffixes, context[s*8:])
			prefixes = append(prefixes, context[:len(context)-s*8])
		}

		return prefixes, suffixes
	}

	delim := byte(0)
	hasDelim := false

	if bytes.IndexByte(context, 0) >= 0 {
		delim = 0
		hasDelim = true
	}

	if !hasDelim && bytes.IndexByte(context, '/') >= 0 {
		delim = '/'
		hasDelim = true
	}

	if !hasDelim && bytes.IndexByte(context, '_') >= 0 {
		delim = '_'
		hasDelim = true
	}

	if hasDelim {
		var splits []int

		for i, b := range context {
			if b == delim {
				splits = append(splits, i)
			}
		}

		if len(splits) > 0 {
			for _, idx := range splits {
				if idx+1 < len(context) {
					suffixes = append(suffixes, context[idx+1:])
				}
			}

			for i := len(splits) - 1; i >= 0; i-- {
				idx := splits[i]

				if idx > 0 {
					prefixes = append(prefixes, context[:idx])
				}
			}

			return prefixes, suffixes
		}
	}

	half := len(context) / 2

	if half > 0 {
		prefixes = append(prefixes, context[:half])
		suffixes = append(suffixes, context[half:])
	}

	return prefixes, suffixes
}

/*
argmax reduces densities through the canonical Argmax primitive, preserving
the winning value's ordinal.
*/
func argmax(densities []float64) (probability.ArgmaxResult, bool) {
	reduction := probability.NewArgmax()
	var result probability.ArgmaxResult
	found := false

	for out := range reduction.Next(sequence.NewValues(densities...).Next(nil)) {
		result = *(*probability.ArgmaxResult)(out)
		found = true
	}

	return result, found
}

/*
evidenceShare reads one member's normalized share through the canonical
composition.
*/
func evidenceShare(densities []float64, index int) (float64, error) {
	selection := equation.NewEvidenceShare(index)
	var share float64

	for out := range selection.Next(sequence.NewOne(unsafe.Pointer(&densities)).Next(nil)) {
		share = *(*float64)(out)
	}

	return share, selection.Error()
}

/*
shannonAmbiguity reads the normalized Shannon entropy of the densities
through the canonical streaming reduction.
*/
func shannonAmbiguity(densities []float64) (float64, error) {
	reduction := probability.NewShannonAmbiguity()
	var ambiguity float64

	for out := range reduction.Next(sequence.NewValues(densities...).Next(nil)) {
		ambiguity = *(*float64)(out)
	}

	return ambiguity, reduction.Error()
}
