package cognition

import (
	"bytes"
	"math"
	"sort"
	"sync/atomic"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/types"
)

const (
	maxBasinCandidates   = 7  // Canonical precursor moments: enter_long, hold_long, exit_long, enter_short, hold_short, exit_short, wait
	maxSensoryCandidates = 16 // Sensory transition hypothesis space
	maxCandidates        = maxBasinCandidates
)

type classAccumulator struct {
	names  [][]byte
	logits []types.Scalar
	counts []uint64
	orders []int
	count  int
}

func (acc *classAccumulator) add(name []byte, logP types.Scalar, count uint64, order int) {
	for i := 0; i < acc.count; i++ {
		if bytes.Equal(acc.names[i], name) {
			if order > acc.orders[i] || (order == acc.orders[i] && logP > acc.logits[i]) {
				acc.logits[i] = logP
				acc.counts[i] = count
				acc.orders[i] = order
			}
			return
		}
	}

	acc.names = append(acc.names, name)
	acc.logits = append(acc.logits, logP)
	acc.counts = append(acc.counts, count)
	acc.orders = append(acc.orders, order)
	acc.count++
}


type Engine struct {
	cfg         Config
	root        atomic.Pointer[iradix.Tree[[]byte]]
	stepCounter atomic.Uint64
	decayFactor float64
}

func NewEngine(cfg Config) *Engine {
	e := &Engine{
		cfg:         cfg,
		decayFactor: cfg.DecayFactor(),
	}

	e.root.Store(iradix.New[[]byte]())
	return e
}

/* Root is the current immutable trie. Observe publishes the next one. */
func (e *Engine) Root() *iradix.Tree[[]byte] {
	return e.root.Load()
}

/*
Observe registers context -> class in the existing packed basin. Without feedback,
it observes a positive association. Optional signed feedback is a dimensionless
reinforcement amount: positive strengthens, negative inhibits, zero only observes
sensory context. It is not stored in a separate reward model.
*/
func (e *Engine) Observe(context []byte, class []byte, feedback ...float64) {
	if len(context) == 0 {
		return
	}

	// Keys are namespaced: b/<class>/<context> for basins; s/<context> for sensory transitions
	basinKey := makeBasinKey(class, context)
	sensoryKey := makeSensoryKey(context)

	var valBuf [WeightSize]byte

	for {
		oldRoot := e.root.Load()
		step := e.stepCounter.Add(1)
		txn := oldRoot.Txn()

		// A zero grade observes the context without reinforcing an action.
		if len(class) > 0 && (len(feedback) == 0 || feedback[0] != 0) {
			weight := PackedWeight{Probability: 1, WriteStep: step}

			if len(feedback) > 0 {
				weight.Probability = 0.5 // Neutral between reinforcement and inhibition.
			}

			if existing, found := oldRoot.Get(basinKey); found {
				weight = DecodeWeight(existing).Effective(step, e.decayFactor)
			}
			weight.Count++
			weight.WriteStep = step
			weight.Reinforce(feedback...)
			weight.Encode(valBuf[:])
			txn.Insert(basinKey, bytes.Clone(valBuf[:]))
		}

		// 2. Update Sensory Suffix Transition
		sState := PackedWeight{Count: 1, Probability: 1.0, WriteStep: step}
		if existing, found := oldRoot.Get(sensoryKey); found {
			prior := DecodeWeight(existing).Effective(step, e.decayFactor)
			sState.Count = prior.Count + 1
			sState.Probability = prior.Probability + (1.0-prior.Probability)/(float64(sState.Count)+1.0)
		}
		sState.Encode(valBuf[:])
		txn.Insert(sensoryKey, bytes.Clone(valBuf[:]))

		newRoot := txn.Commit()
		if e.root.CompareAndSwap(oldRoot, newRoot) {
			break
		}
	}
}

/*
Evaluate performs single-pass classification, ambiguity gating, surprisal calculation, and lookahead.
*/
func (e *Engine) Evaluate(context []byte) Evaluation {
	if len(context) == 0 {
		return Evaluation{Surprisal: e.cfg.SurprisalBreakBits, IsBreak: true}
	}

	root := e.root.Load()
	step := e.stepCounter.Load()

	// -------------------------------------------------------------
	// 1. Attractor Basin Softmax & Contrast (Nomagique Probability)
	// -------------------------------------------------------------
	var names [maxCandidates][]byte
	var logits [maxCandidates]types.Scalar
	var counts [maxCandidates]uint64
	var orders [maxCandidates]int
	acc := classAccumulator{names: names[:0], logits: logits[:0], counts: counts[:0], orders: orders[:0]}
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

		state := DecodeWeight(v).Effective(step, e.decayFactor)
		denom := float64(state.Count) + e.cfg.DirichletAlpha*float64(maxCandidates)
		smoothedP := (float64(state.Count)*state.Probability + e.cfg.DirichletAlpha) / denom
		logP := types.Scalar(math.Log(smoothedP))
		acc.add(class, logP, state.Count, e.cfg.MaxBackoffOrder)
	}

	// Fallback: if no exact match, scan basin keys with backoff matchOrder
	if acc.count == 0 {
		basinPrefix := []byte("b/")
		fallbackIt := root.Root().Iterator()
		fallbackIt.SeekPrefix(basinPrefix)

		for k, v, ok := fallbackIt.Next(); ok; k, v, ok = fallbackIt.Next() {
			if !bytes.HasPrefix(k, basinPrefix) {
				break
			}
			class, basinSeq, valid := parseBasinKey(k)
			if !valid {
				continue
			}

			order := matchOrder(context, basinSeq, e.cfg.MaxBackoffOrder)

			if order > 0 {
				state := DecodeWeight(v).Effective(step, e.decayFactor)
				denom := float64(state.Count) + e.cfg.DirichletAlpha*float64(maxCandidates)
				smoothedP := (float64(state.Count)*state.Probability + e.cfg.DirichletAlpha) / denom
				logP := types.Scalar(math.Log(smoothedP))
				acc.add(class, logP, state.Count, order)
			}
		}
	}

	eval := Evaluation{}

	if acc.count > 0 {
		// Softmax Logit Normalization
		maxLogit := acc.logits[0]
		for i := 1; i < acc.count; i++ {
			if acc.logits[i] > maxLogit {
				maxLogit = acc.logits[i]
			}
		}

		// Lift to unnormalized positive densities for EvidenceShare
		densities := make([]types.Scalar, acc.count)
		for i := 0; i < acc.count; i++ {
			density := math.Exp(float64(acc.logits[i] - maxLogit))
			densities[i] = types.Scalar(density * float64(acc.orders[i]) / float64(e.cfg.MaxBackoffOrder))
		}

		unobservedCount := maxCandidates - acc.count

		if unobservedCount > 0 {
			baseDenom := float64(acc.counts[0]) + e.cfg.DirichletAlpha*float64(maxCandidates)
			unseenSmoothed := e.cfg.DirichletAlpha / baseDenom
			unseenLogit := types.Scalar(math.Log(unseenSmoothed))
			unseenDensity := types.Scalar(
				math.Exp(float64(unseenLogit-maxLogit)) / float64(e.cfg.MaxBackoffOrder),
			)

			for range unobservedCount {
				densities = append(densities, unseenDensity)
			}
		}

		// Use Nomagique's canonical Argmax reduction
		winnerIdx, _, hasWinner := probability.Argmax(densities)
		if hasWinner {
			if winnerIdx < acc.count {
				eval.WinnerClass = string(acc.names[winnerIdx])
				eval.Support = acc.counts[winnerIdx]
			}
			// Use Nomagique's canonical EvidenceShare reduction
			eval.Confidence = float64(probability.EvidenceShare(densities, winnerIdx))

			// Ambiguity: Normalized Shannon entropy in [0, 1] across classes
			eval.Ambiguity = float64(probability.ShannonAmbiguity(densities))

			// Contrast: Calculate log-odds divergence against runner-up
			runnerUpIdx := -1
			var secondBest types.Scalar = -1.0
			for i := 0; i < len(densities); i++ {
				if i == winnerIdx {
					continue
				}
				if densities[i] > secondBest {
					secondBest = densities[i]
					runnerUpIdx = i
				}
			}
			if runnerUpIdx >= 0 {
				if runnerUpIdx < acc.count {
					eval.RunnerUp = string(acc.names[runnerUpIdx])
				}
				if runnerUpIdx >= acc.count {
					eval.RunnerUp = "prior"
				}
				runnerUpShare := float64(probability.EvidenceShare(densities, runnerUpIdx))
				if runnerUpShare > 0 && eval.Confidence > 0 {
					eval.Contrast = math.Log2(eval.Confidence / runnerUpShare)
				}
			}
		}
	}


	// -------------------------------------------------------------
	// 2. Surprisal & Sequence-Break Detection
	// -------------------------------------------------------------
	sensoryKey := makeSensoryKey(context)
	if raw, found := root.Get(sensoryKey); found {
		state := DecodeWeight(raw).Effective(step, e.decayFactor)
		if state.Probability > 0 {
			eval.Surprisal = -math.Log2(state.Probability)
		} else {
			eval.Surprisal = e.cfg.SurprisalBreakBits
		}
	} else {
		// Unseen transition: surprisal derives from Dirichlet baseline over sensory space
		eval.Surprisal = -math.Log2(e.cfg.DirichletAlpha / (1.0 + e.cfg.DirichletAlpha*float64(maxSensoryCandidates)))
	}
	eval.IsBreak = eval.Surprisal >= e.cfg.SurprisalBreakBits

	// -------------------------------------------------------------
	// 3. Multi-Hop Lookahead (Beam Search)
	// -------------------------------------------------------------
	eval.Lookahead = e.beamSearch(root, context, e.cfg.BeamWidth, e.cfg.MaxHops)

	return eval
}

/*
beamSearch explores continuation paths using the underlying radix tree iterator.
*/
func (e *Engine) beamSearch(root *iradix.Tree[[]byte], prefix []byte, width, hops int) []LookaheadPath {
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

				state := DecodeWeight(v)
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
Suffix/prefix matching order: exact (4), prefix (2), suffix (1)
Prefix matches dominant features; suffix matches secondary features.
*/
func matchOrder(context, target []byte, maxOrder int) int {
	if bytes.Equal(context, target) {
		return maxOrder
	}
	if bytes.HasPrefix(context, target) {
		return maxOrder / 2
	}
	if bytes.HasSuffix(context, target) {
		return 1
	}
	return 0
}

func makeBasinKey(class, context []byte) []byte {
	buf := make([]byte, 2+len(context)+1+len(class))
	buf[0] = 'b'
	buf[1] = '/'
	copy(buf[2:], context)
	buf[2+len(context)] = '/'
	copy(buf[3+len(context):], class)
	return buf
}

func makeSensoryKey(context []byte) []byte {
	buf := make([]byte, 2+len(context))
	buf[0] = 's'
	buf[1] = '/'
	copy(buf[2:], context)
	return buf
}

func parseBasinKey(k []byte) ([]byte, []byte, bool) {
	if len(k) < 4 || k[0] != 'b' || k[1] != '/' {
		return nil, nil, false
	}
	rem := k[2:]
	idx := bytes.LastIndexByte(rem, '/')
	if idx <= 0 || idx == len(rem)-1 {
		return nil, nil, false
	}
	return rem[idx+1:], rem[:idx], true
}

func MakeBasinKey(class, context []byte) []byte {
	return makeBasinKey(class, context)
}

func ParseBasinKey(k []byte) ([]byte, []byte, bool) {
	return parseBasinKey(k)
}
