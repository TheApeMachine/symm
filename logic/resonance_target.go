package logic

import "math"

/*
resonanceTarget builds the supervised forward-return label for the resonance task
head (the third "V" head) with a per-symbol adaptive horizon.

The label for a settle at time t is a geometric-decay-weighted sum of the token's
future one-step returns:

	label(t) = Σ_k γ^k · r[t+1+k]

so the horizon is soft and continuous rather than a fixed tick count. γ adapts per
symbol from the observed inter-sample price movement: a token whose price moves a
lot per sample (data-dense, fast) gets a lower γ (tighter horizon), while a quiet
token gets a γ near the ceiling (longer reach). This gives each token a horizon
matched to its own dynamics instead of an arbitrary global N.

Supervision is lagged: a settle's physical input and latent are held until enough
future has accrued that the remaining decay tail is negligible, at which point the
sample is "mature" and its (input, label) pair is emitted for a training replay.
*/
type resonanceTarget struct {
	// gammaMin/gammaMax bound the adaptive decay. maturityTail is the residual
	// decay weight below which a sample is considered fully observed.
	gammaMin     float64
	gammaMax     float64
	maturityTail float64
	// scale normalises raw returns into the tanh-friendly [-1, 1] band the head
	// is trained through, so a typical move maps to a usable target magnitude.
	scale float64

	symbols map[string]*symbolTargetState
}

type resonanceSample struct {
	input     []float64
	price     float64
	weightSum float64 // Σ γ^k accumulated so far for this sample's label
	labelSum  float64 // Σ γ^k · r[t+1+k] accumulated so far
	steps     int
}

type symbolTargetState struct {
	pending  []resonanceSample
	lastMove float64 // EMA of |one-step return|, drives the adaptive γ
	seen     bool
}

/*
maturedSample is an (input, label) pair whose forward window has fully accrued and
is ready to train the task head via a replay.
*/
type maturedSample struct {
	input []float64
	label float64
}

func newResonanceTarget() *resonanceTarget {
	return &resonanceTarget{
		gammaMin:     0.3,
		gammaMax:     0.95,
		maturityTail: 0.02,
		scale:        50.0, // ~2% move → tanh(1.0); returns are small fractions
		symbols:      map[string]*symbolTargetState{},
	}
}

/*
gammaFor derives the per-symbol decay from the smoothed absolute one-step move.
More movement per sample → lower γ (tighter horizon); quieter tokens → γ toward
the ceiling. Falls back to the ceiling before any movement is observed.
*/
func (target *resonanceTarget) gammaFor(state *symbolTargetState) float64 {
	if !state.seen || state.lastMove <= 0 {
		return target.gammaMax
	}

	// Map the smoothed move through a soft response: a 1% typical move lands near
	// the midpoint of the γ band, larger moves compress the horizon further.
	activity := math.Tanh(state.lastMove * target.scale)
	gamma := target.gammaMax - activity*(target.gammaMax-target.gammaMin)

	return math.Max(target.gammaMin, math.Min(target.gammaMax, gamma))
}

/*
Observe records a new settle for a symbol and returns any samples whose forward
window has now fully accrued (ready to train the head). input is the physical
feature vector fed to the resonance solver for this settle; price is the token's
current price used to form future one-step returns.
*/
func (target *resonanceTarget) Observe(
	symbol string,
	input []float64,
	price float64,
) []maturedSample {
	state, ok := target.symbols[symbol]
	if !ok {
		state = &symbolTargetState{}
		target.symbols[symbol] = state
	}

	matured := make([]maturedSample, 0)

	if len(state.pending) > 0 && price > 0 {
		previous := state.pending[len(state.pending)-1].price
		gamma := target.gammaFor(state)

		if previous > 0 {
			ret := (price - previous) / previous

			// Track activity for the adaptive horizon.
			const moveAlpha = 0.1
			if !state.seen {
				state.lastMove = math.Abs(ret)
				state.seen = true
			} else {
				state.lastMove = (1-moveAlpha)*state.lastMove + moveAlpha*math.Abs(ret)
			}

			// Accrue this realised step into every still-open pending sample's
			// decayed label, then release any that have matured.
			survivors := state.pending[:0]

			for i := range state.pending {
				sample := &state.pending[i]
				weight := math.Pow(gamma, float64(sample.steps))
				sample.labelSum += weight * ret
				sample.weightSum += weight
				sample.steps++

				// The remaining tail weight is γ^steps; once negligible the sample
				// is fully observed.
				if math.Pow(gamma, float64(sample.steps)) <= target.maturityTail {
					label := sample.labelSum
					if sample.weightSum > 0 {
						label = math.Tanh(sample.labelSum * target.scale)
					}

					matured = append(matured, maturedSample{
						input: sample.input,
						label: label,
					})
				} else {
					survivors = append(survivors, *sample)
				}
			}

			state.pending = survivors
		}
	}

	// Bound memory: a symbol that never matures (e.g. price stuck) must not grow
	// unbounded. Cap the pending window; drop the oldest if exceeded.
	const maxPending = 512
	if len(state.pending) >= maxPending {
		state.pending = state.pending[1:]
	}

	state.pending = append(state.pending, resonanceSample{
		input: append([]float64(nil), input...),
		price: price,
	})

	return matured
}
