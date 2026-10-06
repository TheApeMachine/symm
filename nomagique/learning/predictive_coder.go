package learning

import (
	"fmt"
	"iter"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
)

/*
PredictiveCoder learns to forecast a transform of a reference series from a
feature vector, by settling a resonance manifold over the features and
training its supervised head against outcomes that actually arrived.

It is an orchestration over three surviving components — the manifold, the
target transform, and the adaptive pace controller — and holds no learning
mathematics of its own.

Predictions are resolved causally: the forecast issued at one step is scored
only once the outcome arrives, so the head is never trained against a target
it was allowed to see.

Each arrival is *[2][]float64 {features, {reference, hasReference, step, time}}.
Reference is the series the target transform is computed over, and
hasReference states whether a usable prior reference existed — a first
observation has nothing to forecast against. Step orders observations so a
prediction issued at t can be resolved against the outcome at t+1.

Each arrival yields *[12][]float64. [0]..[7] are the manifold's settled
reading (see ResonanceManifold); [8] is the head's forecast at the supported
horizon (6 values per row); [9] is the forward latent retention; and

	[10] {supportedHorizon, calibrated, resolvedSteps, pending, confidence,
	     alpha, hasResolution, resolutionHorizon, resolutionPrediction,
	     resolutionTarget, resolutionError, resolutionStep}
	[11] forward curve, one value per supported horizon

Calibrated states whether the head has resolved enough predictions for its
readings to mean anything; until then a consumer must not treat the forecast
as evidence. Every row is freshly allocated per arrival, so a retained curve
never changes underneath its holder.
*/
type PredictiveCoder struct {
	*core.PrimitiveError
	manifold core.Primitive
	target   core.Primitive
	pace     core.Primitive
	ledger   core.Primitive
	alpha    float64
	learn    bool
	horizon  int
	pending  float64
	resolved float64
	ledgerAt [9]float64
	command  [3][]float64
	out      [12][]float64
}

/*
NewPredictiveCoder composes a coder over a manifold sized by the declared
architecture.

The supervised head forecasts a single scalar per horizon, so the target
dimension is one, and maxHorizon becomes the number of independent horizon
models the head holds. Readout selects what the task head harvests as its
features; every horizon holds a covariance matrix quadratic in that width.

The learning rate is never invented here: it comes from the supplied pace
Primitive, which derives it from how badly the manifold is reconstructing its
own input. A nil pace gets the default controller rather than a fabricated
constant. An empty architecture is refused at the first arrival.
*/
func NewPredictiveCoder(
	arch []int,
	maxHorizon int,
	target core.Primitive,
	pace core.Primitive,
	initialAlpha float64,
	learn bool,
	readout ReadoutMode,
) core.Primitive {
	coder := &PredictiveCoder{
		PrimitiveError: core.NewPrimitiveError(),
		target:         target,
		pace:           pace,
		alpha:          initialAlpha,
		learn:          learn,
		horizon:        max(maxHorizon, 1),
	}

	if coder.alpha == 0 {
		coder.alpha = 0.03
	}

	if coder.pace == nil {
		coder.pace = NewPace(coder.alpha, 0.005, 0.150, 0.1, 0.2, 256)
	}

	if len(arch) == 0 {
		return coder
	}

	coder.manifold = NewResonanceManifold(arch, 1, coder.horizon, coder.alpha, readout)
	coder.ledger = NewTemporalLedger(coder.horizon, coder.manifold, coder.target)

	return coder
}

func (coder *PredictiveCoder) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				coder.Error(core.ErrShape)
				return
			}

			input := (*[2][]float64)(arriving)
			features := input[0]
			var context [4]float64
			copy(context[:], input[1])
			reference, hasReference, step := context[0], context[1] != 0, context[2]

			// The manifold refuses an architecture it cannot build, so a nil
			// here is a rejected configuration surfacing at its first use.
			if coder.manifold == nil {
				coder.Error(fmt.Errorf(
					"%w: learning: predictive coder has no manifold: the architecture was rejected",
					core.ErrShape,
				))
				return
			}

			if len(features) == 0 {
				coder.Error(fmt.Errorf(
					"%w: learning: predictive coder requires a feature vector",
					core.ErrShape,
				))
				return
			}

			learnFlag := 0.0

			if coder.learn {
				learnFlag = 1
			}

			advance := 1 - learnFlag
			var settled [10][]float64
			coder.command = [3][]float64{{ManifoldBatch, learnFlag, advance}, features, nil}

			for out := range coder.manifold.Next(data.NewValue(coder.command)) {
				settled = *(*[10][]float64)(out)
			}

			if err := coder.manifold.Error(); err != nil {
				coder.Error(err)
				return
			}

			// The pace controller reads how badly the manifold is reconstructing
			// its own input and sets the learning rate from it, so the rate is
			// derived rather than configured.
			for out := range coder.pace.Next(data.NewValue(settled[0][3])) {
				coder.alpha = (*(*[4]float64)(out))[0]
			}

			if err := coder.pace.Error(); err != nil {
				coder.Error(err)
				return
			}

			coder.command = [3][]float64{{ManifoldAlpha, coder.alpha}, nil, nil}

			for out := range coder.manifold.Next(data.NewValue(coder.command)) {
				settled = *(*[10][]float64)(out)
			}

			if err := coder.manifold.Error(); err != nil {
				coder.Error(err)
				return
			}

			if hasReference && reference > 0 {
				if coder.learn {
					coder.command = [3][]float64{{LedgerResolve, step, reference}, nil, nil}

					for out := range coder.ledger.Next(data.NewValue(coder.command)) {
						coder.ledgerAt = *(*[9]float64)(out)
					}

					if err := coder.ledger.Error(); err != nil {
						coder.Error(err)
						return
					}

					coder.pending = coder.ledgerAt[2]
					coder.resolved = coder.ledgerAt[1]
				}

				coder.command = [3][]float64{{ManifoldReading}, nil, nil}

				for out := range coder.manifold.Next(data.NewValue(coder.command)) {
					settled = *(*[10][]float64)(out)
				}

				if err := coder.manifold.Error(); err != nil {
					coder.Error(err)
					return
				}

				if len(settled[3]) > 0 && len(settled[1]) > 0 {
					coder.command = [3][]float64{
						{LedgerIssue, step, reference, float64(coder.horizon)},
						settled[1],
						settled[3],
					}

					for out := range coder.ledger.Next(data.NewValue(coder.command)) {
						coder.ledgerAt = *(*[9]float64)(out)
					}

					if err := coder.ledger.Error(); err != nil {
						coder.Error(err)
						return
					}

					coder.pending = coder.ledgerAt[2]
				}
			}

			summary := make([]float64, 12)
			summary[2] = coder.resolved
			summary[3] = coder.pending
			summary[5] = coder.alpha
			copy(summary[6:], coder.ledgerAt[3:])

			// The supported horizon is the CONTIGUOUS run of rows whose skill
			// is established, counted from the nearest. A gap ends it: a
			// distant row that happens to have seen data is not reachable
			// evidence if the rows before it have not.
			for horizon := 1; horizon <= coder.horizon; horizon++ {
				if horizon > len(settled[5]) || settled[5][horizon-1] == 0 {
					break
				}

				summary[0] = float64(horizon)

				// Confidence is the skill at the nearest horizon: how much of
				// the target's variation the head explains.
				if horizon == 1 {
					summary[4] = settled[4][0]
				}
			}

			supported := int(summary[0])

			if supported > 0 {
				summary[1] = 1
			}

			// The curve runs exactly as far as the head has learned. Rolling
			// out the full declared depth would append untrained rows, which
			// emit near-zero and read downstream as a genuine flat forecast
			// rather than as absent evidence.
			coder.out = [12][]float64{}
			copy(coder.out[:8], settled[:8])
			coder.command = [3][]float64{{ManifoldForecast, float64(max(supported, 1))}, nil, nil}

			for out := range coder.manifold.Next(data.NewValue(coder.command)) {
				coder.out[8] = (*(*[10][]float64)(out))[8]
			}

			if err := coder.manifold.Error(); err != nil {
				coder.Error(err)
				return
			}

			if supported > 0 {
				coder.out[11] = make([]float64, supported)

				for index := 0; index < supported && index*6 < len(coder.out[8]); index++ {
					coder.out[11][index] = coder.out[8][index*6]
				}

				coder.command = [3][]float64{{ManifoldRetention, float64(supported)}, nil, nil}

				for out := range coder.manifold.Next(data.NewValue(coder.command)) {
					coder.out[9] = (*(*[10][]float64)(out))[9]
				}

				if err := coder.manifold.Error(); err != nil {
					coder.Error(err)
					return
				}
			}

			coder.out[10] = summary

			if !yield(unsafe.Pointer(&coder.out)) {
				return
			}
		}
	}
}
