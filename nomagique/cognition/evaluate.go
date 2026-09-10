package cognition

import (
	"bytes"
	"iter"
	"math"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/probability"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
EvaluateInput is the store as it stands and the question being asked.
*/
type EvaluateInput struct {
	Tree       *iradix.Tree[[]byte]
	Evaluation Evaluation
}

/*
Evaluate reads what the store associates with one precursor sequence.
*/
type Evaluate struct {
	core.Base[EvaluateInput, Evaluation]
}

func NewEvaluate() *Evaluate {
	return &Evaluate{}
}

func (op *Evaluate) Next(
	in iter.Seq[core.Primitive[EvaluateInput, EvaluateInput]],
) iter.Seq[core.Primitive[Evaluation, Evaluation]] {
	return func(yield func(core.Primitive[Evaluation, Evaluation]) bool) {
		for arriving := range in {
			reading, err := op.Recall(arriving.Read())

			if err != nil {
				op.Error(err)
				return
			}

			if !yield(op.Carrier(reading)) {
				return
			}
		}
	}
}

func (op *Evaluate) Recall(input EvaluateInput) (Evaluation, error) {
	if input.Tree == nil {
		return Evaluation{}, core.ErrNotHeld
	}

	held := input.Evaluation
	config := held.Config.normalised()
	classes := make([][]byte, 0, maxCandidates)
	logits := make([]types.Scalar, 0, maxCandidates)
	counted := make([]uint64, 0, maxCandidates)
	orders := make([]int, 0, maxCandidates)
	basin := []byte("b/")
	iterator := input.Tree.Root().Iterator()
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

		logit := types.Scalar(math.Log(smoothed))
		gathered := false

		for index, existing := range classes {
			if bytes.Equal(existing, class) {
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
		return held, nil
	}

	densities := make([]types.Scalar, len(logits))
	largest := logits[0]

	for _, logit := range logits[1:] {
		if logit > largest {
			largest = logit
		}
	}

	for index, logit := range logits {
		density := math.Exp(float64(logit - largest))
		densities[index] = types.Scalar(
			density * float64(orders[index]) / float64(config.MaxBackoffOrder),
		)
	}

	winner, _, chosen := probability.Argmax(densities)

	if !chosen {
		return held, nil
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

	return held, nil
}
