# Discovery and prospective learning

Run `make run`. It starts the default paper-only backend and the learning
dashboard at http://127.0.0.1:3000/learning. The original dashboard stays at `/`;
the sidebar and command palette link to both. Ctrl+C stops both processes. `CONFIG=path`
selects a configuration file through the same command. Existing Kraken market
credentials and the configured paper account remain the sources of market
access, starting cash, fee schedule and instrument rules.
Startup constructs the typed configuration after reading that file, so workload
capacity and other configured producer settings apply to the running owners.

This checkout uses the adjacent `../datura` module through `go.mod`. Its cognitive
inference caches reuse counts only for the same storage revision and decay clock,
and episodic recall only for the same immutable episode snapshot. Parsed sequence
tokens are interned, and recency weights are computed once per rank.
The episodic subtree's own revision preserves that snapshot across unrelated
model writes. These caches remove repeated scans and parsing without dropping
observations or changing their weights. Keep that adjacent checkout available
when running or building this work.

## One numerical owner

`cmd.gridNode` consumes the eleven canonical signal measurement slots and
projects cognition, resonance and manifold scalar readouts. Its grid update,
policy step and on-demand inspection run on the same consumer, immediately after
the shared signal preparation, category and cognition steps for that event.
Witness and viewer publication complete that event turn. Keeping these dependent
steps together avoids separate polling barriers and prevents a pending cognition
batch from withholding all learner and inspection progress. Disruptor
stages can overlap different envelopes, so the policy cannot be a separate
stage reading mutable grid state. There is one numerical grid and no
Observation transport or per-tick copy of the grid.

Level3 orders remain in the websocket transport and its resident book. Delta-dependent signals run at that transport boundary. Their numerical
measurements and symbol/time notifications enter the workspace. The learner reads that book
through the existing guarded API; it does not transport order arrays. A read
can be newer than its triggering notification, so journal records distinguish
local decision/valuation time from the triggering market timestamp. Raw capture
and notification manifests retain the original transport identity.

The grid records all supplied raw values and a separate presence mask. Signed
changes are scaled by adaptive level dispersion, baseline maturity, measurement
maturity and a signal-power fraction. When a producer supplies SNR, it is used;
otherwise the grid estimates movement-to-dispersion power from its own numeric
history. That does not mark the producer's missing SNR as defined.

A rank-two incremental signed profile sketch estimates relative affinities.
Stable inverse profiles can attract; inconsistent or dissimilar profiles repel.
One present coordinate advances per update by weighted distance-error descent.
Quality influences evidence and resistance to displacement. This is a streaming
approximation, not a claim of globally optimal clustering or lossless geometry.
See [Brand's incremental SVD](https://www.merl.com/publications/docs/TR2006-059.pdf).

## Regions

Current quality-conditioned activity is rasterized into a square with
`ceil(sqrt(number of quantities))` cells per side: one cell per quantity on
average. This resolution and four-neighbor connectivity define the numerical
representation. Equal-height connected plateaus merge, and uphill paths form
watershed basins. Otsu between-class variance retains the stronger basin class;
equal-strength basins remain together. There is no selected number of clusters,
activation threshold or neighborhood radius.

Regions are ordered by strength. A region's identity names the strongest
quantity at its selected peak cell, with deterministic spatial tie resolution;
labels remain provenance. A context's own grid version identifies new evidence,
so another symbol's activity cannot reissue its impulse. Fresh hot observations
can prompt a new action even when the ordered region identities remain the same.
Missing observations supply no activity; this implementation does not infer
unobserved simultaneous movement between asynchronous producers.

## Actions and completed matches

`symm` defines feasible WAIT, ENTER, EXIT and SCALE actions. Quantity candidates
successively bisect the currently executable range down to venue lot and cost
minimums. Bisection is a search basis, not a selected allocation percentage.
The context contains ordered region IDs, a delimiter and dyadic inventory
exposure. The model matches these numeric identities exactly. It neither parses
their semantic names nor merges nearby contexts using an invented tolerance.

The previous action is deliberately not part of that identity. With it, every
decision changed the context it would next be recalled under, so no prior ever
accumulated a second observation, exploration could never leave its
unestimable-variance branch, and the policy lane recalled nothing.

An outcome trains its action at every prefix of that context, and Recall reads
the longest prefix that can state a dispersion of its own, backing off towards
the unconditioned estimate as needed. Ordered region identities jitter, so a
long context still almost never repeats exactly; every prefix carries the same
evidence at a coarser resolution, and precision is used where it has been
earned rather than assumed. This is exact prefix matching at a coarser
resolution, not approximate matching: a returned reading was measured under a
context this one genuinely begins with. `PriorReading.Depth` reports which.

`nomagique.learning.Model` accepts an opaque key, numeric context, comparable
action and an explicit authority in [0,1]. Issue captures those facts before
execution. Resolve updates all completed exact matches with weighted Welford
moments, reliability-weighted variance and Kish support. The weights come from
region activity and its quality at issue time, never from the eventual reward.
One observation defines a mean but cannot define dispersion. Support measures
weight concentration, not independence between outcomes.

Exploration balances issued counts while dispersion is unestimable. Otherwise
it samples around the authority-weighted mean using measured standard error.
This is an empirical Gaussian sampling approximation, not a calibrated Bayesian
posterior. [The Thompson sampling tutorial](https://arxiv.org/abs/1707.02038)
describes the distinction between sampled beliefs and guaranteed calibration.
No exploration bonus, temperature or chosen warmup count is configured.

## Independent accounts and reward

Each symbol has four exploratory account lanes and one policy lane. The four
are independent samplers of the same action vocabulary, not one lane per
action: they explore the same evidence in parallel to raise its rate, and they
are expected to reach similar results. Each starts with its own full copy of
the known cash balance and zero inventory. Lanes do not share capital. Their
profits must not be summed into a purported fundable account.

A lane that has spent its capital on execution costs restarts. It is flat,
cannot afford one venue lot, and every further decision it appears to make is
a forced wait, so it resolves its outstanding decisions against the equity it
actually ended with and begins a new episode on a fresh clone of the same
known balance. Episodes are separate accounts in sequence; the retained total
is a record of what a lane realized, never a balance anyone holds.

An action fixes its requested quantity before its next book notification, and
records the depth it was sized against. Fills model taker IOC execution on the
subsequently available displayed depth, capped at each price by what stood
there when the decision was made: liquidity that arrived afterwards was never
available to that decision, and liquidity since cancelled is a race the order
loses. An unrestricted sweep of the current book wins that race every time and
drifts the lanes optimistic. Partial fills, unfilled-remainder cancellation and
the supplied account fee apply as before.
Exact rational arithmetic preserves mixed price, quantity and cash precisions.
Sizing stops when remaining cash cannot buy one venue lot at the current ask;
all later asks are more expensive and cannot add executable quantity.
The model excludes maker queue position, hidden liquidity, counterfactual
market impact and an exact exchange latency model. It never fabricates missing
depth. An unmarkable account is displayed as unvalued and retried on later
notifications. New risk is not issued while valuation is incomplete.

Account marks include cash and full liquidation of inventory at displayed bids,
including exit fees. External funding is explicitly known to be zero within
these cloned accounts. `strategy.AccountReward` supplies funding-adjusted
cumulative numerical values to the domain-free `learning.RewardLedger`.

Every decision is scored over the same forward window: change from its issue
equity, minus the reward rate known at issue times actual elapsed seconds,
divided by starting capital. The window is `horizonEpochs` multiples of this
market's own measured cadence of impulse change, so a fast instrument is scored
on a fast window; an unmeasured cadence resolves nothing rather than assuming
one. Settlement runs on every book update, and an open position is valued at
executable liquidation prices including its exit fee, so waiting and holding
are measured, not merely permitted.

Waiting and acting must share that window. Resolving a wait on the next book
update because the wallet happened to be flat, while an entry waited for its
position to close, measured two different things and made waiting look
reliably harmless. A decision settled early because its account ran out of
capital is marked truncated in the journal and is not presented as a completed
measurement.

Overlapping targets are correlated; the account ledger counts economic profit
only once. A negative historical reward rate can give inactivity positive
relative feedback while actual cash profit remains zero. There is no invented
inactivity fine forcing loss-making trades, and no claim of causal attribution
from overlapping returns.

The policy lane selects from completed exploration priors without exploratory
orders, and records its own outcomes under the same identity it selects from —
otherwise its experience trains a subtree nothing reads. Positive evidence
influences new exposure through prior authority; absence of positive evidence
leaves a flat policy account waiting. Reductions are feasible without that
scaling.

This owner is the only decision path. The advisor deliberation package, the
planner/opportunity/allocation layer and the stoploss regulator have been
removed rather than left unmounted: an entry carries no protective geometry,
`broker.Position` marks and reports but never closes itself, and an exit
happens because the agent commanded one. A second mechanism that could close a
position would be a second policy, learning nothing and contradicting the one
whose outcomes are being measured.

## Measured skill and going live

`strategy.SkillMeter` estimates the policy lane's forward competence from its
own resolved outcomes, under exponential forgetting at a declared retention
rate, so an edge earned in one regime decays out instead of being averaged
away.

Only decisions covering disjoint forward windows enter that estimate. Decisions
issue far faster than a window closes, so a whole batch of them resolves
against one account valuation with near-identical targets; admitting all of
them reports one observation as many, collapses the measured dispersion and
saturates confidence, which then flips sign with the next batch. A decision is
admitted only when its window began at or after the end of the last admitted
one, and truncated windows are never admitted because they cover less forward
tape than the horizon. The excluded decisions still train their own action
priors — this gate governs the competence estimate that grants execution
authority, not what the agent learns. Promotion requires the mean minus `skillSigma` standard errors to exceed
zero, on effective evidence of at least `skillSigma` squared: an edge must be
larger than its own measurement error, and a short run of similar outcomes
reporting near-zero dispersion cannot read as certainty. A reading below that
floor is reported unqualified and its bound is not displayed at all, rather
than shown as a number an operator could mistake for a measurement. Demotion requires only a
non-positive mean. That asymmetry is deliberate hysteresis.

There are two modes: calibrating, and trading the account. `trading.model`
names which account that is — paper or real — and the agent does not earn its
way from one to the other. Paper and real are the same behaviour against
different accounts; only whether it is trading at all is earned.

Learning never stops. A trading agent keeps exploring and re-estimating on its
own virtual lanes, which is what lets a degrading edge pull it back to
calibrating without any separate supervision. Without an attached
`ExecutionDesk` no account is attached and the agent can never leave
calibrating, so such a run has no code path that reaches an account. This is
continuous use of observed skill, not a claim that a profitability
certification threshold has been crossed.

The account itself is live from the first tick either way. `cmd.learningDesk`
routes policy intents to `broker.Desk`, and `cmd.learningTickNode` steps that
desk and stamps its equity and open positions onto every ticker envelope, so
an operator watching a calibrating agent is watching a real balance rather
than an empty one.

Entries reach the desk self-managed: the agent re-evaluates every open position
on each book update and issues its own EXIT, so it does not hand over a
strategy stop. The desk still sizes the entry under its own risk plan and
retains that plan as a catastrophic floor. Partial reductions have no desk
operation yet and are refused and counted rather than executed as something
else — an account that quietly did a different thing from the one the agent
recorded would corrupt every later measurement drawn from that lane.

`strategy.attribution` accumulates, per hot quantity and action kind, the
outcomes of decisions issued while that quantity was hot. That is the discovery
question — which measurements should determine which actions — answered from
resolved evidence rather than declared by hand. It is association under the
agent's own exploration, not a controlled comparison.

## Forward testing

There is no back test. Replaying history against the current model lets it see
what came next; `cmd.forwardReviewer` instead runs behind the live tape,
discovers the confirmed price excursions on the captured run, and reports them
to `Agent.Review`. The agent compares each against the exposure its policy lane
actually had at the time.

Reviewing is measurement, never training: an outcome discovered after the fact
is never fed back into a decision, because a policy trained on episodes it could
not have seen is a policy leaking the future. An excursion is only confirmed
once price has turned back from its extremum, so the reviewer necessarily lags,
and an unconfirmed episode is not judged at all. "Sat it out" is not a mistake —
the excursion was not visible when the decision was made — but a policy exposed
to none of them has no path to an edge, and that is what the reading is for. An
episode older than the retained exposure history is reported as not reviewable
rather than counted as a miss.

## Account reconciliation

The agent decides from its own simulated wallet, which is not the account. The
two can disagree: an entry on a symbol the account already holds, or an exit on
one it never opened. `cmd.learningDesk` reconciles against the desk's actual
position and counts the disagreement rather than acting on it, and a submission
the account refuses is counted and reported, never fatal. Halting the workload
on one refused order would stop every symbol from learning because a single
venue could not take a single order.

`ExecutionDesk.Submit` must never talk to the venue. The agent runs inside the
workspace consumer that also feeds the terminal, so a REST round-trip taken
there stops that consumer for its whole duration: the dashboard froze the
instant a position opened, because `Desk.Execute` placed the entry order
synchronously on the deciding path. `cmd.learningDesk` now reconciles inline —
a `sync.Map` read — and queues; one worker goroutine places the orders, which
also keeps a symbol's intents in the order the agent issued them. A full queue
drops rather than blocks, because an intent that waited behind a backlog was
priced against a book that no longer exists. `strategy.ExecutionStatus` reports
submitted, diverged, dropped and refused separately, since they are different
facts about the account and only one of them is an error.

Reductions are executed, not approximated. `broker.Position.Reduce` sells part
of an open lot without claiming the exit, because "hold less of this" is not
"stop holding this", and a reduction that closed the whole lot would record an
account state the agent never decided on.

## Inspection and persistence

`/learning` returns a coherent on-demand state for the selected symbol: actual
coordinates, activity, regions, separate wallets and valuation timestamps.
`/learning/events` reads the current run's durable decision journal. Issued,
filled, rejected, waited and resolved records use the existing ordered SQLite
writer into the indexed `learning_events` table after book locks are released. Recorder failure stops learning visibly.
The dashboard refresh interval and journal page size control inspection only;
neither is a learning horizon or an evidence cutoff.

The learner currently starts fresh on each process run. The journal preserves
action and account evidence but is not a model/grid checkpoint, and restart
does not reconstruct learned priors automatically. Virtual results are forward
observations under the stated fill model, not proof of exchange profitability.

## Package boundary

`nomagique` organizes and optimizes numbers. It has no knowledge of market
symbols, wallets, orders, fills, equity or funding, including in its tests.
`symm` owns all execution rules, economic accounting and display vocabulary.
