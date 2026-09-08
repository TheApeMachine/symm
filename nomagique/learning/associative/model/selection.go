package model

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/learning/associative/prior"
	"math/rand/v2"
)

/*
Select compares feasible actions using completed context matches. Exploration
first observes actions whose variance is not estimable, balancing their issued
counts so parallel callers do not all request the same unknown action. Once
dispersion is estimable, Gaussian posterior sampling uses its measured standard
error. This is an empirical sampling approximation, not a calibrated posterior
or an independence claim for correlated inputs. There is no exploration bonus,
temperature or selected warmup count. Without exploration it selects the
authority-weighted mean. The caller supplies a zero-effect action if feasible.
*/
func (model *Model[Key, Action]) Select(
	key Key, context []uint64, actions []Action, explore bool, recall ...func(Key, []uint64, Action) prior.Reading,
) (Action, prior.Reading, error) {
	var selected Action
	var selectedPrior prior.Reading

	if len(actions) == 0 {
		return selected, selectedPrior, errnie.Err(errnie.Validation, "model: feasible actions are required", nil)
	}

	read := model.Recall

	if len(recall) > 0 {
		read = recall[0]
	}

	best := math.Inf(-1)
	unsupported := false
	least := ^uint64(0)
	start := 0

	if explore {
		start = rand.IntN(len(actions))
	}

	for offset := range actions {
		action := actions[(start+offset)%len(actions)]
		record := read(key, context, action)
		score := record.Mean * record.Authority

		if explore && !record.VarianceDefined {
			issued := record.Samples + record.Pending

			if !unsupported || issued < least {
				selected, selectedPrior, least = action, record, issued
			}

			unsupported = true
			continue
		}

		if unsupported {
			continue
		}

		if explore {
			score += rand.NormFloat64() * math.Sqrt(record.SamplingVariance())
		}

		if score > best {
			selected, selectedPrior, best = action, record, score
		}
	}

	return selected, selectedPrior, nil
}
