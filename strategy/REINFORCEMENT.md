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
The context contains ordered region IDs, a delimiter, dyadic inventory exposure,
and the previous action, its refinement and its direction. The model matches
these numeric identities exactly. It neither parses their semantic names nor
merges nearby contexts using an invented tolerance.

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

Each symbol has four exploratory account lanes, corresponding to the four-action
vocabulary, and one paper policy lane. Each starts with its own full copy of the
known cash balance and zero inventory. Lanes do not share capital. Their profits
must not be summed into a purported fundable account.

An action fixes its requested quantity before its next book notification. Fills
model taker IOC execution on the subsequently available displayed depth, with
partial fills, unfilled-remainder cancellation and the supplied account fee.
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

When an account becomes flat, each unresolved action receives its own subsequent
return-to-go: change from its issue equity, minus the reward rate known at issue
times actual elapsed seconds, divided by starting capital. Entry targets thus
include their subsequent execution costs and outcome. Overlapping targets are
correlated; the account ledger counts economic profit only once. A negative
historical reward rate can give inactivity positive relative feedback while
actual cash profit remains zero. There is no invented inactivity fine forcing
loss-making trades, and no claim of causal attribution from overlapping returns.

The paper lane selects from completed virtual priors without exploratory orders.
Positive evidence influences new exposure through prior authority; absence of
positive evidence leaves a flat paper account waiting. Reductions are feasible
without that scaling. This is continuous use of observed skill, not a claim
that a profitability certification threshold has been crossed. Default startup
mounts this owner instead of the former advisor/planner/stoploss execution path.
No exchange orders are sent by these lanes.

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
