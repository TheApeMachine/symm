package cognition

import (
	"iter"
	"sync/atomic"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/statistic"
)

/*
LookaheadPath is one scored future branch trajectory.
*/
type LookaheadPath struct {
	Sequence string
	Score    float64
}

/*
ClassCandidate carries evidence recalled for one specific action or category class.
*/
type ClassCandidate struct {
	Name        string
	Probability float64
	Support     uint64
}

/*
Evaluation is the full information-theoretic readout of an evaluated context.
It is plain wire payload: zero methods.
*/
type Evaluation struct {
	Context     []byte
	Step        uint64
	Support     uint64
	WinnerClass string
	RunnerUp    string
	Confidence  float64
	Contrast    float64
	IsTie       bool
	Candidates  []ClassCandidate
	Surprisal   float64
	Ambiguity   float64
	IsBreak     bool
	Lookahead   []LookaheadPath
}

/*
Evaluator coordinates the atomic cognition primitives (Attractor, Classification,
Ambiguity, Surprisal, Lookahead) and empirical Welford surprisal moments for break detection.
*/
type Evaluator struct {
	*core.PrimitiveError
	attractor        *Attractor
	classification   *Classification
	ambiguity        *probability.Ambiguity
	surprisal        *Surprisal
	lookahead        *Lookahead
	surprisalMoments statistic.Moments
	stepCounter      *atomic.Uint64
	out              Evaluation
}

func NewEvaluator(trie *Trie) *Evaluator {
	return &Evaluator{
		PrimitiveError: core.NewPrimitiveError(),
		attractor:      NewAttractor(&trie.Root),
		classification: NewClassification(),
		ambiguity:      probability.NewAmbiguity(),
		surprisal:      NewSurprisal(&trie.Root, &trie.StepCounter),
		lookahead:      NewLookahead(&trie.Root),
		stepCounter:    &trie.StepCounter,
	}
}

func (evaluator *Evaluator) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if evaluator.Error() != nil {
			return
		}

		for arriving := range in {
			if arriving == nil {
				continue
			}

			context := *(*[]byte)(arriving)
			if len(context) == 0 {
				continue
			}

			inCtx := func(yieldCtx func(unsafe.Pointer) bool) {
				yieldCtx(unsafe.Pointer(&context))
			}

			// 1. Basin seek -> 2. Classification
			var classResult ClassificationResult
			for outCls := range evaluator.classification.Next(evaluator.attractor.Next(inCtx)) {
				classResult = *(*ClassificationResult)(outCls)
			}

			// 3. Shannon Ambiguity
			var ambiguity float64
			if len(classResult.Candidates) > 0 {
				inAmb := func(yieldAmb func(unsafe.Pointer) bool) {
					for i := range classResult.Candidates {
						p := classResult.Candidates[i].Probability
						if !yieldAmb(unsafe.Pointer(&p)) {
							return
						}
					}
				}
				for outAmb := range evaluator.ambiguity.Next(inAmb) {
					ambiguity = *(*float64)(outAmb)
				}
			}

			// 4. Surprisal
			var surprisal float64
			for outSurp := range evaluator.surprisal.Next(inCtx) {
				surprisal = *(*float64)(outSurp)
			}

			// 5. Sequence Break via running empirical dispersion
			var isBreak bool
			if surprisal > 0 {
				reading := evaluator.surprisalMoments.Update(surprisal)
				if reading.VarianceDefined && reading.Dispersion > 0 {
					isBreak = surprisal > reading.Prior.Mean+reading.Dispersion
				}
			}

			// 6. Lookahead Traversal
			var lookaheadPaths []LookaheadPath
			for outPath := range evaluator.lookahead.Next(inCtx) {
				lookaheadPaths = append(lookaheadPaths, *(*LookaheadPath)(outPath))
			}

			var step uint64
			if evaluator.stepCounter != nil {
				step = evaluator.stepCounter.Load()
			}

			evaluator.out = Evaluation{
				Context:     context,
				Step:        step,
				Support:     classResult.Support,
				WinnerClass: classResult.WinnerClass,
				RunnerUp:    classResult.RunnerUp,
				Confidence:  classResult.Confidence,
				Contrast:    classResult.Contrast,
				IsTie:       classResult.IsTie,
				Candidates:  classResult.Candidates,
				Surprisal:   surprisal,
				Ambiguity:   ambiguity,
				IsBreak:     isBreak,
				Lookahead:   lookaheadPaths,
			}

			if !yield(unsafe.Pointer(&evaluator.out)) {
				return
			}
		}
	}
}
