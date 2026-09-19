package cognition

import "iter"

type Evaluation func() (
	winner []byte,
	runnerUp []byte,
	confidence float64,
	contrast float64,
	support uint64,
	surprisal float64,
	isBreak bool,
	ambiguity float64,
	lookaheadPaths iter.Seq2[[]byte, float64],
)
