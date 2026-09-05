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
	key Key, context []uint64, actions []Action, explore bool,
) (Action, PriorReading, error) {
	var selected Action
	var selectedPrior PriorReading

	if len(actions) == 0 {
		return selected, selectedPrior, errnie.Err(errnie.Validation, "model: feasible actions are required", nil)
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
		prior := model.Recall(key, context, action)
		score := prior.Mean * prior.Authority

		if explore && !prior.VarianceDefined {
			issued := prior.Samples + model.inflight(key, context, action)

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
			score += rand.NormFloat64() * math.Sqrt(prior.Variance/prior.Support)
		}

		if score > best {
			selected, selectedPrior, best = action, prior, score
		}
	}

	return selected, selectedPrior, nil
}

/* inflight reads the count stored with an action's interned prior. */
func (model *Model[Key, Action]) inflight(key Key, context []uint64, action Action) uint64 {
	node := model.contexts[key]

	for _, token := range context {
		if node == nil {
			return 0
		}
		node = node.children[token]
	}

	if node == nil || node.priors[action] == nil {
		return 0
	}
	return node.priors[action].pending
}
