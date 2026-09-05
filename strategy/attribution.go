package strategy

import (
	"slices"

	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

/*
attributionKey names one measurement identity together with one action kind.
The identity is a region token: the strongest contributing numeric quantity at
a hot basin's peak, which is a grid column and therefore a specific signal or
solver readout. Nothing here interprets the quantity's meaning.
*/
type attributionKey struct {
	token uint64
	kind  types.Action
}

/*
MetricInfluence is the measured association between one numeric quantity being
hot at decision time and the outcome of one action kind taken while it was.

This is exactly the discovery question — which measurements should determine
which actions — answered from resolved evidence rather than declared by hand.
It is an association under the agent's own exploration, not a controlled
comparison: a quantity that is hot whenever the tape moves will appear in the
context of good and bad outcomes alike, and overlapping return windows make
these observations correlated. Read it as where evidence has accumulated, not
as proof that the quantity caused the outcome.
*/
type MetricInfluence struct {
	Token  uint64                `json:"token"`
	Source string                `json:"source"`
	Label  string                `json:"label"`
	Action string                `json:"action"`
	Prior  learning.PriorReading `json:"prior"`
}

/*
attribution accumulates per-quantity, per-action outcome evidence as decisions
resolve. It shares the weighted Welford estimator the action priors use, so a
quantity cannot gain influence from the size of an outcome — only from the
observation authority fixed when the decision issued.
*/
type attribution struct {
	priors map[attributionKey]*learning.Prior
}

/* observe credits every quantity that was hot when this decision was issued. */
func (store *attribution) observe(
	tokens []uint64, kind types.Action, target, authority float64,
) error {
	if store.priors == nil {
		store.priors = make(map[attributionKey]*learning.Prior)
	}

	for _, token := range tokens {
		key := attributionKey{token: token, kind: kind}
		prior := store.priors[key]

		if prior == nil {
			prior = &learning.Prior{}
			store.priors[key] = prior
		}

		if err := prior.Observe(target, authority); err != nil {
			return err
		}
	}

	return nil
}

/*
report copies the accumulated evidence, strongest authority first, resolving
each token back to the grid column identity it names. Only readings with
estimable dispersion are ranked ahead of the rest: a single observation
defines a mean but says nothing about whether that mean is distinguishable
from noise.
*/
func (store *attribution) report(columns [][2]string) []MetricInfluence {
	report := make([]MetricInfluence, 0, len(store.priors))

	for key, prior := range store.priors {
		influence := MetricInfluence{
			Token: key.token, Action: string(key.kind), Prior: prior.Reading(),
		}

		if index := int(key.token) - 1; index >= 0 && index < len(columns) {
			influence.Source, influence.Label = columns[index][0], columns[index][1]
		}

		report = append(report, influence)
	}

	slices.SortFunc(report, func(left, right MetricInfluence) int {
		if left.Prior.Authority != right.Prior.Authority {
			if left.Prior.Authority > right.Prior.Authority {
				return -1
			}
			return 1
		}

		if left.Token != right.Token {
			if left.Token < right.Token {
				return -1
			}
			return 1
		}

		if left.Action < right.Action {
			return -1
		}

		if left.Action > right.Action {
			return 1
		}

		return 0
	})

	return report
}
