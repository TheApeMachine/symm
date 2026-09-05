package learning

import (
	"math"
	"math/rand/v2"

	"github.com/theapemachine/errnie"
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
	key Key, context []uint64, actions []Action, explore bool, recall ...func(Key, []uint64, Action) PriorReading,
) (Action, PriorReading, error) {
	var selected Action
	var selectedPrior PriorReading

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
		prior := read(key, context, action)
		score := prior.Mean * prior.Authority

		if explore && !prior.VarianceDefined {
			issued := prior.Samples + prior.Pending

			if !unsupported || issued < least {
				selected, selectedPrior, least = action, prior, issued
			}

			unsupported = true
			continue
		}

		if unsupported {
			continue
		}

		if explore {
			effectiveSupport := prior.Support

			if len(context) > 0 && prior.Depth < len(context) {
				depthGap := float64(len(context) - prior.Depth)
				effectiveSupport = prior.Support / (1.0 + depthGap)
			}

			if effectiveSupport < 1.0 {
				effectiveSupport = 1.0
			}

			variance := prior.Variance

			if variance < 0 {
				variance = 0
			}

			score += rand.NormFloat64() * math.Sqrt(variance/effectiveSupport)
		}

		if score > best {
			selected, selectedPrior, best = action, prior, score
		}
	}

	return selected, selectedPrior, nil
}
