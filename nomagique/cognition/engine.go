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
	"errors"
	"fmt"
	"io"
	"iter"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"unsafe"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/transport"
)

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
	err         error
	out         Result
	cfg         Config
	root        atomic.Pointer[iradix.Tree[[]byte]]
	stepCounter atomic.Uint64
	decayFactor float64
	classCounts sync.Map
}

/*
NewEngine instantiates the cognitive engine Primitive. Unset bounds in the
configuration fold to the declared defaults; the normalized values are
computed here and owned by the engine.
*/
func NewEngine(cfg Config) core.Primitive {
	engine := &Engine{
		cfg:         cfg.normalised(),
		decayFactor: cfg.normalised().decayFactor(),
	}

	engine.root.Store(iradix.New[[]byte]())
	return engine
}

/*
Next executes each arriving command and yields its result. An invalid command
is recorded in Error and ends the stream.
*/
func (op *Engine) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	if op.err != nil {
		return func(yield func(unsafe.Pointer) bool) {}
	}

	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			command := (*Command)(arriving)
			result, err := op.execute(command)

			if err != nil {
				op.Error(err)
				return
			}

			op.out = result

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}

/*
Error records the first error it sees and joins any subsequent errors to it.
*/
func (op *Engine) Error(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			op.err = errors.Join(op.err, err)
		}
	}

	return op.err
}

/*
execute dispatches one command to its intent and returns its result.
*/
func (op *Engine) execute(command *Command) (Result, error) {
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
		return op.observe(*command.Observe)
	}

	if command.Evaluate != nil {
		return op.evaluate(command.Evaluate.Context)
	}

	if command.Snapshot != nil {
		return op.snapshot()
	}

	if command.Restore != nil {
		return op.restore(command.Restore)
	}

	if command.Census != nil {
		return Result{Classes: op.census()}, nil
	}

	return Result{Tree: op.root.Load()}, nil
}

/*
observe registers context -> class in the existing packed basin and publishes
the next immutable root through compare-and-swap. Keys are namespaced:
b/<context>/<class> for basins; s/<context> for sensory transitions.
*/
func (op *Engine) observe(assoc Association) (Result, error) {
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
		oldRoot := op.root.Load()
		step := op.stepCounter.Add(1)
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
				weight = decodeWeight(existing).effective(step, op.decayFactor)
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
			prior := decodeWeight(existing).effective(step, op.decayFactor)
			sState.Count = prior.Count + 1
			sState.Probability = prior.Probability + (1.0-prior.Probability)/(float64(sState.Count)+1.0)
		}

		encodeWeight(valBuf[:], sState)
		txn.Insert(sensoryKey, bytes.Clone(valBuf[:]))

		newRoot := txn.Commit()

		if op.root.CompareAndSwap(oldRoot, newRoot) {
			if isNew {
				op.incrementClass(string(assoc.Class))
			}

			return Result{Tree: newRoot}, nil
		}
	}
}

/*
evaluate performs single-pass classification, ambiguity gating, surprisal
calculation, and lookahead.
*/
func (op *Engine) evaluate(context []byte) (Result, error) {
	if len(context) == 0 {
		return Result{Evaluation: Evaluation{
			Surprisal: op.cfg.SurprisalBreakBits,
			IsBreak:   true,
		}}, nil
	}

	root := op.root.Load()
	step := op.stepCounter.Load()

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

		state := decodeWeight(v).effective(step, op.decayFactor)
		mass := float64(state.Count) * state.Probability
		acc.add(class, mass, state.Count, op.cfg.MaxBackoffOrder)
	}

	// Fallback: if no exact match, use prefix and suffix backoff via direct SeekPrefix
	if acc.count == 0 {
		maxSteps := op.cfg.MaxBackoffOrder

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
				if searchSubPrefix(root, sub, step, op.decayFactor, 1, op.cfg.MaxBackoffOrder, &acc) {
					break
				}
			}
		}

		// Check prefixes if no suffix matched
		if acc.count == 0 {
			order := max(1, op.cfg.MaxBackoffOrder/2)

			for _, sub := range prefixes {
				if searchSubPrefix(root, sub, step, op.decayFactor, order, op.cfg.MaxBackoffOrder, &acc) {
					break
				}
			}
		}
	}

	eval := Evaluation{Context: context, Step: step}

	if acc.count > 0 {
		reading, err := op.classify(&acc)
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
		state := decodeWeight(raw).effective(step, op.decayFactor)

		prob := (float64(state.Count) + op.cfg.DirichletAlpha) / (totalSteps + op.cfg.DirichletAlpha*float64(maxSensoryCandidates))
		if prob > 0 {
			eval.Surprisal = -math.Log2(prob)
		} else {
			eval.Surprisal = op.cfg.SurprisalBreakBits
		}
	} else {
		// Unseen transition: surprisal derives from Dirichlet baseline over sensory space
		eval.Surprisal = -math.Log2(op.cfg.DirichletAlpha / (totalSteps + op.cfg.DirichletAlpha*float64(maxSensoryCandidates)))
	}

	eval.IsBreak = eval.Surprisal >= op.cfg.SurprisalBreakBits

	// -------------------------------------------------------------
	// 3. Multi-Hop Lookahead (Beam Search)
	// -------------------------------------------------------------
	eval.Lookahead = beamSearch(root, context, op.cfg.BeamWidth, op.cfg.MaxHops)

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
func (op *Engine) classify(acc *classAccumulator) (classification, error) {
	var reading classification

	if acc.count == 0 {
		return reading, nil
	}

	totalMass := 0.0
	for i := 0; i < acc.count; i++ {
		totalMass += acc.masses[i]
	}

	distinctTotal := op.distinctClasses()
	k := max(acc.count+1, distinctTotal)
	unobservedCount := k - acc.count

	alpha := op.cfg.DirichletAlpha
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
func (op *Engine) census() map[string]int32 {
	res := make(map[string]int32)
	op.classCounts.Range(func(key, value any) bool {
		if k, ok := key.(string); ok {
			if cnt, ok := value.(*atomic.Int32); ok {
				res[k] = cnt.Load()
			}
		}

		return true
	})

	return res
}

func (op *Engine) distinctClasses() int {
	count := 0
	op.classCounts.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

func (op *Engine) incrementClass(class string) {
	val, _ := op.classCounts.LoadOrStore(class, &atomic.Int32{})

	if cnt, ok := val.(*atomic.Int32); ok {
		cnt.Add(1)
	}
}

/*
snapshot serializes one immutable trie snapshot, its configuration and clock.
There is no checkpoint model: the values are the same packed weights read by
evaluate. Storage I/O belongs to the caller.
*/
func (op *Engine) snapshot() (Result, error) {
	root := op.root.Load()
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)

	for _, value := range []any{"cognition/packed-weight/1", op.cfg, op.stepCounter.Load(), root.Len()} {
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
func (op *Engine) restore(encoded []byte) (Result, error) {
	if op.root.Load().Len() != 0 || op.stepCounter.Load() != 0 {
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

		if err := op.validate(key, value, step); err != nil {
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

	op.cfg = config
	op.decayFactor = config.decayFactor()
	op.stepCounter.Store(step)
	root := transaction.Commit()
	op.root.Store(root)

	return Result{Tree: root}, nil
}

/*
validate rejects records that are neither packed basins nor sensory
transitions, and weights outside their domain.
*/
func (op *Engine) validate(key, value []byte, step uint64) error {
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

func (acc *classAccumulator) add(name []byte, mass float64, count uint64, order int) {
	for i := 0; i < acc.count; i++ {
		if bytes.Equal(acc.names[i], name) {
			if order > acc.orders[i] || (order == acc.orders[i] && mass > acc.masses[i]) {
				acc.masses[i] = mass
				acc.counts[i] = count
				acc.orders[i] = order
			}

			return
		}
	}

	acc.names = append(acc.names, name)
	acc.masses = append(acc.masses, mass)
	acc.counts = append(acc.counts, count)
	acc.orders = append(acc.orders, order)
	acc.count++
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
	} else if bytes.IndexByte(context, '/') >= 0 {
		delim = '/'
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
	reduction := transport.NewEvaluate(probability.NewArgmax())
	var result probability.ArgmaxResult
	found := false

	for out := range reduction.Next(transport.NewValues(densities...).Next(nil)) {
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
	selection := transport.NewEvaluate(equation.NewEvidenceShare(index))
	var share float64

	for out := range selection.Next(transport.NewOne(unsafe.Pointer(&densities)).Next(nil)) {
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

	for out := range reduction.Next(transport.NewValues(densities...).Next(nil)) {
		ambiguity = *(*float64)(out)
	}

	return ambiguity, reduction.Error()
}
