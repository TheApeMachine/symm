package cognition

import (
	"bytes"
	"math"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Evaluate reads what the store associates with one precursor sequence.

It is configured with the sequence being asked about and handed the store as it
stands, and answers with everything the store has to say: which class it
associates most strongly, how much of the collective evidence that class holds,
how far ahead of the next class it is, and how spread the rest of the evidence
was.

Evidence is gathered across every stored sequence the current one reaches. An
exact match is the full order — this is the situation, not one resembling it. A
stored sequence the current one ends with is the same recent past arrived at
through a different history, which is what lets a sequence never seen in full be
answered for by ones that were. A stored sequence the current one merely opens
with is the weakest reading that is still a reading.

A link's raw strength is not its standing. An unsmoothed link observed once
reports certainty, so evidence is weighed against a pseudo-count prior standing
for the possibility that a sequence taught nothing at all.
*/
type Evaluate struct {
	core.PrimitiveError
	current core.Primitive
}

func NewEvaluate(state core.Primitive) *Evaluate {
	return &Evaluate{current: state}
}

func (evaluate *Evaluate) Next(in core.Primitive) core.Primitive {
	return core.Yield(
		evaluate.current,
		in,
		func(held Evaluation, arriving *iradix.Tree[[]byte]) Evaluation {
			if arriving == nil {
				evaluate.Error(core.ErrNotHeld)

				return held
			}
			config := held.Config.normalised()
			classes := make([][]byte, 0, maxCandidates)
			logits := make([]types.Scalar, 0, maxCandidates)
			counted := make([]uint64, 0, maxCandidates)
			orders := make([]int, 0, maxCandidates)
			basin := []byte("b/")
			iterator := arriving.Root().Iterator()
			iterator.SeekPrefix(basin)

			for key, value, more := iterator.Next(); more; key, value, more = iterator.Next() {
				class, stored, named := parseBasinKey(key)

				if !named {
					continue
				}
				order := matchOrder(held.Context, stored, config.MaxBackoffOrder)

				if order == 0 {
					continue
				}
				weight := DecodeWeight(value).Effective(held.Step, config.DecayFactor())
				denominator := float64(weight.Count) + config.DirichletAlpha*maxCandidates
				smoothed := (float64(weight.Count)*weight.Probability + config.DirichletAlpha) / denominator

				if smoothed <= 0 {
					continue
				}
				// A link's evidence is its own strength. Scaling it by how well
				// it matched would make a closer match score worse, because the
				// strength is a share and its logarithm is negative: an exact
				// match multiplied by four falls below a passing resemblance
				// multiplied by one.
				logit := types.Scalar(math.Log(smoothed))
				gathered := false

				for index, existing := range classes {
					if bytes.Equal(existing, class) {
						// The situation most like this one decides, and a closer
						// match beats a stronger one that resembles it less.
						// Adding every resemblance instead lets a class seen in
						// many loosely similar situations outweigh one seen in
						// precisely this situation, which is frequency answering
						// a question that was about recognition.
						if order > orders[index] ||
							(order == orders[index] && logit > logits[index]) {
							orders[index] = order
							logits[index] = logit
							counted[index] = weight.Count
						}
						gathered = true

						break
					}
				}

				if !gathered && len(classes) < maxCandidates {
					classes = append(classes, bytes.Clone(class))
					logits = append(logits, logit)
					counted = append(counted, weight.Count)
					orders = append(orders, order)
				}
			}

			if len(classes) == 0 {
				return held
			}
			densities := make([]types.Scalar, len(logits))
			largest := logits[0]

			for _, logit := range logits[1:] {
				if logit > largest {
					largest = logit
				}
			}

			// A class that only resembled the situation does not compete with one
			// that matched it. Its evidence is discounted by how far short of an
			// exact match it fell, so backoff answers when nothing exact was
			// stored without overruling something that was.
			for index, logit := range logits {
				density := math.Exp(float64(logit - largest))
				densities[index] = types.Scalar(
					density * float64(orders[index]) / float64(config.MaxBackoffOrder),
				)
			}
			winner, _, chosen := probability.Argmax(densities)

			if !chosen {
				return held
			}
			held.WinnerClass = string(classes[winner])
			held.Support = counted[winner]
			held.Confidence = float64(probability.EvidenceShare(densities, winner))
			held.Ambiguity = float64(probability.ShannonAmbiguity(densities))
			runnerUp, best := -1, types.Scalar(-1)

			for index, density := range densities {
				if index != winner && density > best {
					runnerUp, best = index, density
				}
			}

			if runnerUp >= 0 {
				held.RunnerUp = string(classes[runnerUp])
				opposing := float64(probability.EvidenceShare(densities, runnerUp))

				if opposing > 0 && held.Confidence > 0 {
					held.Contrast = math.Log2(held.Confidence / opposing)
				}
			}

			return held
		},
		evaluate,
	)
}

func (evaluate *Evaluate) Read() any { return core.To[any](evaluate.current) }
