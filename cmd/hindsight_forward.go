package cmd

import (
	"context"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy"
)

/*
forwardDelay is how far behind the tape the reviewer runs. An excursion is only
an excursion once price has turned back from its extremum, so a reviewer level
with the market would confirm nothing. This is a declared operating choice, not
a measured one, and it bounds only how quickly a completed episode is noticed —
never whether it is noticed.
*/
const forwardDelay = 30 * time.Second

/*
forwardReviewer is the system's substitute for a backtest. Rather than replaying
history against the current model — which lets the model see what came next — it
lets the market run slightly ahead of the reviewer and asks what the tape
actually offered while the agent was deciding without that knowledge.

It reads the same durable capture the Hindsight surfaces read, and reports
confirmed price excursions to the agent, which compares them against what its
policy lane was holding at the time. It never feeds an outcome back into a
decision: reviewing is measurement, and a reviewer that trained the model on
episodes it could not have seen would be leaking the future into the policy.
*/
type forwardReviewer struct {
	engine  *store.SQLite
	learner *strategy.Agent
	runID   hindsight.RunID
	policy  hindsight.DiscoveryPolicy
}

/* newForwardReviewer wires the delay line to its capture and its agent. */
func newForwardReviewer(
	engine *store.SQLite, learner *strategy.Agent, runID hindsight.RunID,
) *forwardReviewer {
	return &forwardReviewer{
		engine: engine, learner: learner, runID: runID,
		policy: hindsight.DefaultDiscoveryPolicy(),
	}
}

/*
Run reviews the tape behind real time until the context ends. A failed pass is
reported and retried on the next tick: the reviewer is an observer, and losing
one pass must not stop the agent it observes.
*/
func (reviewer *forwardReviewer) Run(ctx context.Context) {
	ticker := time.NewTicker(forwardDelay)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := reviewer.pass(ctx); err != nil {
				errnie.Error(errnie.Err(
					errnie.Internal,
					"symm: forward review pass failed",
					err,
				))
			}
		}
	}
}

/*
pass discovers the confirmed episodes on the captured tape and hands them to
the agent. Unconfirmed episodes are left alone: the evidence that ends them has
not arrived, and judging the agent against an excursion that may still be
running would be judging it against a guess.
*/
func (reviewer *forwardReviewer) pass(ctx context.Context) error {
	observations, err := reviewer.engine.ListMarketObservations(string(reviewer.runID))

	if err != nil {
		return err
	}

	if len(observations) == 0 {
		return nil
	}

	index := hindsight.NewRunIndex(reviewer.runID, observations)
	confirmed := make([]hindsight.Episode, 0, len(observations)/64+1)

	for _, summary := range index.Summaries(reviewer.policy) {
		for _, episode := range index.Discover(summary.Symbol, reviewer.policy).Episodes {
			if !episode.Confirmed {
				continue
			}

			if episode.Kind != hindsight.EpisodeUpwardExcursion &&
				episode.Kind != hindsight.EpisodeDownwardExcursion {
				continue
			}

			confirmed = append(confirmed, episode)
		}
	}

	return reviewer.learner.Review(ctx, confirmed)
}
