# Capture, causal grading, and settled region tokens

Status: proposed mathematical design, not an implemented learner. The runtime
and capture repairs accompanying this document do not establish profitability.
`TRAINING.md` remains the behavioral contract. Two choices below need review:
lexicographic versus additive sympathy, and exact-event versus early-warning
labels. Neither is concealed in an implementation constant.

## Runtime repair boundary

A capability without both `write` and `done` is a resource. Compile constructs
it, binds it into consumers, retains it across compatible recompiles, and
releases it. It has no evaluation result, root, or scheduled invocation.

Excursion returns `none | move {anchor, ignition, extremum, excursion,
confirmed}`. Diagnostics remain available on both branches. Only the move
branch activates move-field routes. The old `found` plus default scalar values
is removed. Anchor remains an event-mining boundary; it is not training point A.

Capture and offline mining now have separate manifests. The previous training
manifest no longer reinforces predicted winners or runs TaskLearner,
TemporalLedger, Surprisal, Window/PageRank, or a four-rank token surrogate.
It mines single-series excursions only (`data.0.last` on a ticker frame); it is
not a multi-symbol miner. Symbol partitioning and all-record expansion are still
required. There is no claim that an absent remapper has settled.
The paper process must not be advertised as implemented until model publication,
position accounting, and the causal evaluation tests below exist.

## Tape contract

Socket receipt supplies endpoint, receive time, and untouched frame bytes.
Capture owns a session identifier and monotonically increasing frame identity.
It serializes the binary field as base64 in its JSON row; the table decodes that
field into Arrow Binary. It must never store the envelope as the raw frame.
Metadata absent from the producer stays absent. A frame may contain multiple
symbols or be a control response; do not assign it an invented single symbol.

The repaired capture manifest uses `raw_frames_v2`: existing null-payload rows
cannot be recovered by changing the reader. It requires receive time, endpoint,
frame identity, and payload. Symbol and kind remain optional until a
protocol-owned metadata extractor supplies them. No frame is discarded because
it is not a market update.

Storage owns append cadence through its byte budget. Explicit `Durable.flush`
persists the tail before release, using a context independent of stop signals.
An ambiguous append retains its rows. Shutdown currently flushes rows already
accepted by storage; ingress stop/drain coordination is still required to cover
frames queued in the socket when cancellation occurs. Before archive training, scan must order
by an explicit tape cursor and deduplicate matching capture identities; scan
file order is not a global market order. Conflicting duplicate identities must
fail, not select one payload. Receive time currently stored as Iceberg timestamp
has microsecond resolution; add a lossless receive-time/cursor field before
merging independently captured streams. Do not pretend timestamp ties establish
causal order.

Subscription configuration belongs to ingress. A subscription must be sent once
per connection and restored on reconnect, not sent on every graph evaluation.
Actual symbols and authenticated Level 3 configuration are deployment inputs.
The capture manifest does not invent them.

## Ground-truth owner and bootstrap

Mining outputs event identities and tape cursors: symbol, precursor start,
B (ignition), C (extremum), and D (confirmation cursor). B/C price alone cannot
retrieve fragments or grade predictions. D may be later than C; this distinction
must remain visible. Live grading cannot occur until D.

A separate fragment sampler samples A uniformly from the eligible observed
cursors in [precursor start, B). Use an unbiased bounded random draw, with the
seed and selected A recorded in the fragment identity. No eligible cursor means
no entry fragment, not A=B. Preserve actual timestamps and instrument identity.

Proposed initial label contract is exact-event classification:

- Before B: WAIT while flat.
- At B: ENTER while flat.
- Between B and C: WAIT while holding.
- At C: EXIT while holding.
- A fully resolved no-event fragment: WAIT throughout.

WAIT means retain position, so position state must accompany the causal token
sequence. A down-leg must specify its inventory interpretation; do not teach
ENTER for a fall in a long-only process. Unsupported/inapplicable inventory
transitions are ungraded rather than relabeled WAIT.

This contract detects event onset, not arbitrary early entry. If the product
requires an ENTER *before* B, specify an execution-derived lead interval (for
example measured order latency and executable quotes). Do not silently label
all of A→B ENTER or choose a tolerance in bars. Report cursor/elapsed-time lead
as measured quantities without widening class boundaries.

A grader takes the recorded causal sequence and the fragment's truth, separately
from the predicted class. Every resolved labeled example increments its truth
class in Memory, even if Memory predicted nothing. That is the bootstrap.
Prediction accuracy is a separate counter; reinforcing only correct guesses
cannot bootstrap an empty trie. This intentionally amends the current sentence
in TRAINING.md about reinforcing only correct predictions.

Recording the same fragment/cursor/model-input-version twice must not count as
two observations. Persist an example identity and checkpoint atomically with
memory updates. Random A selections do not make repeated copies of the same
underlying labeled observation independent evidence.

## Sparse metric observations

Grid registration owns stable metric identity and original coordinates. A
missing slot is not zero and is not automatically the latest value of that
metric. Maintain per-metric sufficient statistics on its own arrivals; retain
observation cursors and support with each reading.

Pair observations must have an explicit alignment rule. Use overlapping
observation intervals for asynchronous metrics, following the existing
Hayashi–Yoshida ownership where applicable. A feed that has not arrived during
an interval is unknown, not a demonstrated failure to react. Only a closed
interval with an observed non-response supports the specification's absence
repulsion. Counting transport sparsity as disagreement would learn the feed
schedule instead of market relationships.

For paired, resolved reaction intervals define sign products q in {-1,0,+1}.
Zero represents an observed non-response, not missing input. Retain counts,
signed sum Q, and squared sum S. For N>1, a candidate sign-consistency statistic
is (Q²-S)/(N(N-1)), the mean product over distinct interval pairs. It is positive
for consistently same or opposite movement and penalizes alternating sign
relationships. A separate observed-nonresponse rate must be reported: the
consistency statistic alone only dilutes absence; it does not implement the
required repulsion. No token may call that incomplete statistic a full remapper.

Magnitude compatibility uses dimensionless magnitudes normalized by each
metric's measured RMS on the same paired support. The bounded pairwise match
is 1-|u-v|/(u+v); the denominator must be nonzero. This is a proposed geometric
similarity, not a probability. Unavailable scale means unavailable comparison.

Authority must come from the metric's existing SNR, maturity and uncertainty
owner. It must not be synthesized from PageRank. Before choosing a formula,
resolve whether current SNR is an amplitude ratio or a power ratio and what
maturity estimates; multiplying undocumented numbers is not a mathematical
specification. Emit these quantities with their provenance into the UI.

## Finite remapping and a structural settling gate

Use a finite grid of available cells and an injective mapping of metric IDs to
cells. Grid dimensions are representational capacity, not a market window.
Freeze one evidence revision for a solve. Never update pair weights halfway
through the solve.

A concrete settling mechanism is deterministic strict-descent permutation
search. Evaluate legal cell moves/swaps against an explicit objective. Accept
only strict improvements; break candidate ties by stable metric identities.
After a full pass with no improving move, publish the mapping as a local fixed
point. A finite permutation space plus strict improvement terminates without a
chosen epsilon, iteration horizon, or confidence multiplier. An interrupted
solve stays unsettled. A local fixed point is not a claim of global optimality.

Proposed objective ordering is: signed co-movement compatibility at short
distance; then magnitude compatibility; then authority-weighted displacement
from the previous settled map. Use normalized squared cell distance so grid
size does not silently set a market threshold. Displacement penalized by each
cell's authority makes movement of weak cells cheaper than movement of strong
cells. Zero/undefined authority cannot anchor another cell.

Lexicographic comparison removes arbitrary weights between these quantities,
but differs from a strictly additive interpretation of TRAINING.md. That choice
must be accepted or replaced with a data-derived common-unit objective before
implementation. Also specify the nonresponse repulsion term identified above.
These are remaining mathematical decisions, not parameters to guess in code.

Region extraction follows the settled authority landscape using watershed
basins, deterministic plateau handling, and boundaries at saddles. Stable
region identity comes from its member metric IDs, not a transient cell/rank
index. A changed membership means a new token vocabulary version.

Select hot basins using a distribution-derived split, such as Otsu's weighted
between-class variance on basin activation, with authority as documented
weights. If no nonempty split exists, emit no hot-region token. No fixed count
of four, fallback threshold, or widening to fill empty classes is allowed.

Publish `unsettled | settled {revision, vocabulary, regions, token}`. Reinforce
requires the settled branch, valid truth, and a sequence whose vocabulary is
consistent. A later map must not retroactively rewrite earlier token history.

## Programs and proof

1. Capture: real subscriptions → ingress metadata → capture envelope → Iceberg;
   no model evaluation and no replay input.
2. Train: ordered deduplicated archive → miner/sampler → causal replay → settled
   remapper → truth grader → shared Memory. Future B/C/D travel only on grader
   inputs. Persist immutable model snapshots and input vocabulary metadata.
3. Paper: live ingress → the published frozen vocabulary/model → virtual fills
   and decimal accounting. Subsequent truth grades those recorded decisions;
   it never enters the original selection inputs.

Required acceptance cases: fresh-memory bootstrap; incorrect predictions still
record truth; repeated examples do not inflate support; A varies while B/C stay
fixed; changed future labels do not change precursor tokens; empty/non-event
fragments; asynchronous and absent signals; consistently inverted reactions;
inconsistent signs; unequal authority; remapping ties and interrupted solves;
empty region splits; vocabulary changes; duplicate/reordered tape rows; partial
and failed durable flushes; B→C exit examples; process restart and recovery.
