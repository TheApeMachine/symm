package cmd

import (
	"context"
	"slices"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/system"
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

	observations = slices.DeleteFunc(observations, func(observation hindsight.Observation) bool { return observation.Domain != "spot" })
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

/*
warmupEpisodes teaches the agent from excursions the retained record already
confirmed, before the live loop starts.

The live reviewer only sees its own run, and an excursion is only confirmed once
price has turned back — so a short run learns nothing and a restart discards
every opportunity the desk ever saw. The retained record has those moves already
on it.

It is deliberately bounded. Discovery reads the captured tape, and an unbounded
walk back through every retained run at boot would compete with the capture
writer for exactly the reason that walk was bounded everywhere else.

The runs it spends that budget on are the ones holding the most journalled
context, not the most recent. A string of short restarts is the newest thing on
the record and holds nothing worth learning from, while the run that captured a
real move may be days older. What was not reached is reported rather than being
left to look like an absence of opportunities.
*/
func warmupEpisodes(engine *store.SQLite, learner *strategy.Agent) strategy.EpisodeWarmup {
	report := strategy.EpisodeWarmup{}

	if engine == nil || learner == nil {
		return report
	}
	runs, err := engine.ContextfulRuns(system.Cfg.Learning.WarmupRuns)

	if err != nil {
		errnie.Error(err)
		return report
	}
	policy := hindsight.DefaultDiscoveryPolicy()

	for _, run := range runs {
		observations, err := engine.ListMarketObservations(string(run))

		if err != nil {
			errnie.Error(err)
			continue
		}

		if len(observations) == 0 {
			continue
		}

		observations = slices.DeleteFunc(observations, func(observation hindsight.Observation) bool {
			return observation.Domain != "spot"
		})
		contexts, err := engine.RetainedContexts(run, system.Cfg.Learning.WarmupContexts)

		if err != nil {
			errnie.Error(err)
			continue
		}

		if len(contexts) == 0 {
			continue
		}
		index := hindsight.NewRunIndex(run, observations)
		confirmed := make([]hindsight.Episode, 0)

		for _, summary := range index.Summaries(policy) {
			for _, episode := range index.Discover(summary.Symbol, policy).Episodes {
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

		/*
			The historical spread is not retained, so the round trip accounted
			for here is the fee on both legs. It understates the true cost by
			whatever the spread was, and the evidence is net of that much
			rather than of nothing.
		*/
		learned := learner.WarmupEpisodes(confirmed, contexts, learner.RoundTripFees)
		report.Runs++
		report.Episodes += learned.Episodes
		report.Trained += learned.Trained
		report.Uncontexted += learned.Uncontexted
		report.Unusable += learned.Unusable

		if learned.LastReason != "" {
			report.LastReason = learned.LastReason
		}
	}

	return report
}
