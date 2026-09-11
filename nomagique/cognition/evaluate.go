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
	exactPrefix := make([]byte, 2+len(held.Context)+1)
	exactPrefix[0] = 'b'
	exactPrefix[1] = '/'
	copy(exactPrefix[2:], held.Context)
	exactPrefix[2+len(held.Context)] = '/'

	iterator := input.Tree.Root().Iterator()
	iterator.SeekPrefix(exactPrefix)

	for key, value, more := iterator.Next(); more; key, value, more = iterator.Next() {
		if !bytes.HasPrefix(key, exactPrefix) {
			break
		}
		class, _, named := parseBasinKey(key)

		if !named {
			continue
		}

		order := config.MaxBackoffOrder
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
		basin := []byte("b/")
		fallbackIt := input.Tree.Root().Iterator()
		fallbackIt.SeekPrefix(basin)

		for key, value, more := fallbackIt.Next(); more; key, value, more = fallbackIt.Next() {
			if !bytes.HasPrefix(key, basin) {
				break
			}
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

	unobservedCount := maxCandidates - len(classes)

	if unobservedCount > 0 {
		// Prior density for each unobserved candidate in the Dirichlet hypothesis space.
		// Smoothed prior mass ensures single observations do not claim 100% certainty.
		baseDenom := float64(counted[0]) + config.DirichletAlpha*maxCandidates
		unseenSmoothed := config.DirichletAlpha / baseDenom
		unseenLogit := types.Scalar(math.Log(unseenSmoothed))
		unseenDensity := types.Scalar(
			math.Exp(float64(unseenLogit-largest)) / float64(config.MaxBackoffOrder),
		)

		for range unobservedCount {
			densities = append(densities, unseenDensity)
		}
	}

	winner, _, chosen := probability.Argmax(densities)

	if !chosen {
		return held, nil
	}

	if winner < len(classes) {
		held.WinnerClass = string(classes[winner])
		held.Support = counted[winner]
	}

	held.Confidence = float64(probability.EvidenceShare(densities, winner))
	held.Ambiguity = float64(probability.ShannonAmbiguity(densities))
	runnerUp, best := -1, types.Scalar(-1)

	for index, density := range densities {
		if index != winner && density > best {
			runnerUp, best = index, density
		}
	}

	if runnerUp >= 0 {
		if runnerUp < len(classes) {
			held.RunnerUp = string(classes[runnerUp])
		}

		if runnerUp >= len(classes) {
			held.RunnerUp = "prior"
		}
		opposing := float64(probability.EvidenceShare(densities, runnerUp))

		if opposing > 0 && held.Confidence > 0 {
			held.Contrast = math.Log2(held.Confidence / opposing)
		}
	}

	return held, nil
}

