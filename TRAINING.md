Yes. I’d make the prompt explicit enough that the agent cannot quietly collapse “training” into one vague loop, and cannot satisfy the frontend requirement with decorative counters that are not wired to real state.

Here’s the prompt I’d use.

---

# SYMM — Temporal Precursor Learning, Staged Training, and Forward Proof

Implement the next learning cut of SYMM.

This task starts only after the current subtractive architecture cleanup is complete and `master` is green.

The goal is not to make the system appear intelligent or profitable.

The goal is to implement the intended learning experiment faithfully enough that we can determine whether it actually works.

The central hypothesis is:

> Recurrent temporal fragments of the Impulse Map may contain enough information to recognize the development of economically useful market excursions, avoid bad entries, remain in valid positions while the excursion develops, and recognize when the excursion has ended.

The experiment must include both retrospective model development from captured market tape and genuinely forward paper trading against the real-time market.

If the idea fails, report the failure.

Do not fake success.

---

# 1. Governing principles

## One thing should only ever really be one thing

A semantic concept has one canonical runtime representation and one owner.

Do not add parallel models, duplicate stores, shadow state, compatibility aliases, alternative learner paths, synthetic DTO layers, or adapters whose only purpose is to preserve an obsolete path.

When something is replaced:

1. replace it;
2. migrate all callers;
3. delete the old path.

Do not leave both implementations alive.

Serialization is an encoding boundary, not another domain model.

Frontend state should retain the real payload type it receives.

Do not cast one semantic object into another merely because an existing store expects the wrong type.

---

## No fakery

Never invent information because the real quantity is inconvenient or unavailable.

Specifically prohibited:

- synthetic training outcomes;
- synthetic flat/chop examples;
- fixed positive-return examples;
- fake fills;
- simulated hindsight trades presented as actual trades;
- future data in precursor keys;
- maximum favorable excursion presented as realized return;
- arbitrary thresholds disguised as configuration;
- arbitrary lookback windows;
- arbitrary top-N region counts;
- zero substitution for missing data;
- BTC/USD or any other symbol fallback for unlabeled observations;
- training and evaluation using different representations;
- score smoothing that makes an immature model look stable;
- frontend values that are placeholders for unavailable backend data;
- charts built from invented distributions;
- hardcoded demo values in live surfaces.

Undefined stays undefined.

Missing stays missing.

Quiet tape is not the same as missing tape.

---

# 2. Preserve the existing architecture

This is not permission to invent another learning system.

Use the existing ownership chain:

```text
market measurements
    ↓
Impulse Map
    ↓
regions / Impulse
    ↓
temporal Precursor
    ↓
cognition.Engine predictive radix trie
    ↓
Training
    ↓
Trader
```

Historical development uses the existing Iceberg-backed Hindsight/catalog tape and delayed supervision.

Reuse:

- `strategy/impulse`
- `strategy/precursor.go`
- `strategy/rehearsal.go`
- `hindsight/tables`
- `nomagique/cognition`
- the existing Training dashboard
- the existing paper trading transport
- the existing broker execution path

Do not add:

- another classifier;
- another policy model;
- another trie;
- an HMM;
- a neural sequence model;
- MCTS;
- a reward network;
- an embedding learner;
- a semantic pattern recognizer;
- an “Agent lane” implementation;
- Levenshtein/fuzzy string matching.

The predictive radix trie is the model being tested.

---

# 3. The actual experiment has multiple stages

Training is not one undifferentiated state.

There are distinct stages with distinct evidence and distinct authority.

The frontend must show these stages truthfully.

Use a small explicit stage/state representation owned by Training. Do not infer the stage in the frontend from unrelated counters.

Conceptually:

```text
MODEL DEVELOPMENT
    ↓
HELD-OUT HISTORICAL VALIDATION
    ↓
FORWARD PAPER LEARNING
    ↓
FORWARD SKILL DEMONSTRATED
```

These names may be adjusted if the existing vocabulary provides a better canonical name, but the meanings must remain distinct.

Do not introduce artificial “level 1 / level 2 / level 3” gamification.

---

# 4. Stage A — Model development from Iceberg tape

The first stage develops the representation and predictive memory from previously captured real market tape stored in Iceberg Tables.

This stage exists to develop:

1. the Impulse Map geometry;
2. stable regions;
3. temporal precursor fragments;
4. the predictive radix trie.

No live trading authority exists in this stage.

No paper order is required to produce historical supervision.

The stored market tape is replayed chronologically.

The exact same Impulse Map and precursor encoding used for live inference must be used during replay.

Given identical observations through tick T:

```text
replay key at T == live key at T
```

byte-for-byte.

Test this.

---

# 5. The Impulse Map remains deliberately ignorant

The model does not consume human metric semantics.

Metric names and producers remain available as provenance for humans.

They do not tell the learner what a metric means.

The grid exists because there are hundreds of observations whose individual semantic meaning cannot be known with certainty.

Let empirical behaviour organize them.

The intended relationship remains:

```text
observations that behave sympathetically
    ↓
geometrically related cells
    ↓
watershed regions
    ↓
anonymous market-state tokens
```

Relationship priority remains conceptually:

1. consistent movement relationship;
2. consistency of relative movement magnitude;
3. measurement maturity / SNR controls authority and displacement.

Consistent inverse movement is a valid relationship.

Orientation must survive independently from proximity.

High-quality evidence may make one point more authoritative than another, but quality must not create a relationship where no relationship was measured.

---

# 6. Region selection must remain empirical

Do not select an arbitrary fixed number of active regions.

Remove fixed constructions equivalent to:

```text
minimum 3
maximum 4
top 5
```

unless an external representation genuinely requires that cardinality.

The current strength distribution must determine which regions are distinguishably active.

If the data does not justify separating hot regions from cold regions, preserve that uncertainty.

Do not manufacture a ranking distinction because a consumer expects one.

---

# 7. A precursor is a temporal tape fragment

This is essential.

A precursor is not merely:

```text
[current strongest region,
 current second region,
 current third region]
```

A precursor is:

```text
I₀ → I₁ → I₂ → … → Iₙ
```

where each `I` is one meaningful successive Impulse state for one symbol.

The encoding must preserve:

- the state of one Impulse;
- temporal ordering between Impulses;
- symbol scope;
- holding/non-holding scope where appropriate.

Do not flatten time into another unordered collection.

Do not use human metric names as tokens.

---

# 8. Do not train on scheduler frequency

Repeated identical Impulse states must not generate repeated training evidence simply because the feed produced more envelopes.

Learning concerns development.

If:

```text
I₁ == I₂ == I₃ == I₄
```

in the sense relevant to the precursor representation, that should not become four independent precursor transitions.

Only meaningful state development advances the temporal fragment.

Do not let producer publication rate become evidence weight.

---

# 9. No arbitrary temporal horizon

Do not introduce:

- `last 8 states`;
- `last 16 frames`;
- `last 30 seconds`;
- `maxOrder = 8`;
- fixed history depth;
- fixed market-time lookback.

Historical tape fragments supply natural precursor lengths.

If live memory must be bounded for resource reasons, derive the retained temporal extent from observed fragment/model support or another real structural/resource boundary.

A resource bound may constrain storage.

It must not masquerade as a market belief.

---

# 10. Detect complete tape fragments, not only winners

The historical detector must cover the market tape rather than extracting only attractive upward moves.

The canonical completed fragment classes are:

```text
UP
DOWN
CHOP
FLAT
```

These are empirical tape outcomes, not human market stories.

For the current long-only action space:

- `UP` may support ENTER if the move was economically usable;
- `DOWN` teaches avoidance;
- `CHOP` teaches avoidance;
- `FLAT` teaches avoidance.

This does not mean DOWN is universally undesirable.

It means it is not a valid long-entry target under the current action set.

---

# 11. Flat is a real observation

Distinguish:

```text
invalid / missing evidence
```

from:

```text
valid evidence showing no distinguishable active excursion
```

The latter is FLAT/quiet market information.

It must be representable.

Do not convert missing inputs into flat tape.

Do not discard genuine quiet tape merely because nothing dramatic happened.

A model expected to learn when not to trade requires real quiet examples.

---

# 12. Chop is also real

CHOP must come from actual tape.

Do not create synthetic chop sequences.

Use the same measured movement uncertainty/friction scale used by the detector.

Conceptually:

- UP: positive movement is distinguishably dominant;
- DOWN: negative movement is distinguishably dominant;
- FLAT: neither side produces a distinguishable excursion;
- CHOP: material movement occurs but directional dominance is not stable/distinguishable.

The exact mathematical implementation must derive from measured statistics.

Do not add a hand-selected “chop threshold.”

---

# 13. Fragment boundaries must be causal

For each completed fragment:

```text
A = beginning of observable precursor development
B = causal entry/opportunity boundary
C = causal termination boundary
```

The intended learning problems are:

```text
A → … → B     entry recognition

B → … → C     continuation / exit recognition
```

The future may be used after the fact to identify that a completed fragment existed.

It must never move B or C backward to a price point that could not have been identified causally at the time.

---

# 14. Do not fabricate A

The first fragment of a tape may have no honest precursor start.

If A cannot be established from an earlier observed structural boundary, that fragment is unsupported for precursor training.

Do not create A by:

- subtracting observation counts;
- subtracting timestamps;
- applying an arbitrary window;
- choosing “N frames before B.”

Unsupported is a valid result.

---

# 15. C is not the best price

This is non-negotiable.

Suppose:

```text
tick 100 = excursion extremum
tick 107 = enough evidence exists to detect reversal / exhaustion
```

Then:

```text
C = 107
```

not 100.

The extremum may be retained for diagnostics.

It must not become the EXIT target merely because hindsight knows it was optimal.

---

# 16. Historical economics must be executable economics

For a hypothetical entry at B and causal exit at C:

```text
return =
    executable liquidation value at C
    -
    executable entry cost at B
```

Use:

- actual captured bid/ask;
- actual venue fees;
- actual available executable economics already owned by Price where relevant.

Do not use mid-price fantasy fills.

Do not use the later best price.

Do not claim arbitrary order-size executability from a one-unit quote calculation.

Be precise about what the historical economic measurement represents.

---

# 17. Maximum favorable excursion is diagnostic only

Retain maximum favorable excursion and adverse excursion if useful.

They are diagnostics.

They are never:

- realized P&L;
- policy return;
- EXIT timing;
- evidence that the agent could have captured the peak.

Delete any current logic that substitutes peak profit for B→C causal return.

---

# 18. Recognition and economics are separate questions

Do not turn economic return into a convenient fake classifier.

The trie should learn action boundaries.

Economic evaluation measures whether acting on those recognized boundaries was useful.

Do not train:

```text
ENTER = +profit
WAIT  = -profit
```

merely to manufacture opposing classes.

Use the actual categorical association semantics of the cognition Engine.

Maintain economic statistics separately.

The system must be able to answer independently:

```text
Did the model recognize the correct action boundary?

Was acting on that prediction economically useful?
```

---

# 19. Historical entry supervision

For each valid A→B fragment:

Every proper causal prefix before B teaches:

```text
WAIT
```

because B has not occurred yet.

The complete context at B teaches:

```text
ENTER
```

only if:

1. the completed fragment is an UP opportunity;
2. entering at B and exiting at causal C produces positive executable return after friction;
3. required evidence is fully available.

Otherwise the B context teaches:

```text
WAIT
```

Therefore the training corpus naturally includes:

- DOWN → WAIT;
- CHOP → WAIT;
- FLAT → WAIT;
- upward-but-unprofitable-after-friction → WAIT;
- useful UP → ENTER.

Do not fabricate class balancing.

Measure and report the observed class distribution.

---

# 20. Historical exit supervision

Only construct holding-state exit supervision for a fragment whose B point is itself a valid historical entry target.

From B through C:

Every proper temporal prefix before C teaches:

```text
WAIT
```

under holding scope.

At C:

```text
EXIT
```

This gives the model evidence for both:

```text
keep holding
```

and:

```text
the excursion has ended
```

Do not train EXIT only at C while providing no examples of “not yet.”

Do not train an EXIT context under non-holding scope.

Historical holding state is counterfactual context only:

> “Evaluate what the model should recognize if the B entry had occurred.”

It is not an executed position.

---

# 21. Prequential historical validation

Historical replay must remain chronological.

For every example:

1. reconstruct its causal context;
2. query the model;
3. freeze the prediction;
4. observe the delayed label;
5. score the prediction;
6. only then train the label.

Never train then score the same sample.

Never randomly shuffle historical tape into ordinary train/test rows.

Time is part of the experiment.

---

# 22. Stage B — Held-out historical skill

Model development alone does not grant paper-trading authority.

The training process must expose held-out/prequential skill from the chronological Iceberg replay.

At minimum measure separately:

## Entry

- valid UP opportunities;
- correct ENTER;
- missed ENTER;
- false ENTER on UP that did not clear friction;
- false ENTER on DOWN;
- false ENTER on CHOP;
- false ENTER on FLAT;
- correct WAIT by each negative fragment class;
- first ENTER prediction relative to B.

## Exit

- correct WAIT during B→C continuation;
- premature EXIT;
- correct EXIT at C;
- missed EXIT;
- first EXIT prediction relative to C.

## Economics

For held-out predictions only:

- number of predicted entries;
- cumulative executable B→C return;
- mean return;
- return squared / variance sufficient statistics;
- uncertainty / standard error;
- profitable count if useful;
- losing count if useful.

Do not collapse this into one generic “skill” number.

---

# 23. Historical skill gate

Paper forward learning begins only when the historical model demonstrates measurable skill.

Do not use an arbitrary sample count such as 10, 50 or 100.

The estimator itself may require the mathematically minimal support necessary for its uncertainty to exist.

For economic authority, use a defensible uncertainty-aware criterion such as:

```text
mean held-out return - standard error > 0
```

or an equivalent mathematically justified lower bound already supported by the system.

But economic return alone is not sufficient.

The model must also demonstrate actual boundary recognition rather than earning a positive number through a few lucky cases.

The stage transition should therefore require:

- defined historical held-out entry evidence;
- defined historical held-out exit evidence;
- defined economic uncertainty;
- positive economic lower bound;
- no unresolved model/replay integrity failure.

Do not invent arbitrary confidence percentages.

If evidence is insufficient, the model remains in historical development/validation.

---

# 24. Stage C — Forward paper learning

Once historical held-out skill is genuinely demonstrated, enable PAPER trading against the live real-time market.

This is a different stage.

It must be visually and semantically distinct from replay.

The paper trader receives only information available in real time.

No Hindsight outcome may influence a live action before that outcome occurs.

No catalog replay decision may be presented as a paper trade.

---

# 25. Forward prediction must be frozen before the outcome

For every live paper decision:

Capture the model state/prediction before the future market outcome exists.

At minimum retain enough identity to later associate:

- symbol;
- precursor key/context identity;
- model/trie version or logical step;
- action;
- action time/tick;
- model support/confidence/ambiguity;
- paper order/fill identity if action was executed.

Do not retrospectively recompute “what the model would have predicted.”

Score what it actually predicted.

---

# 26. Paper trading must use real paper execution

Use the existing paper trading execution path.

A paper ENTER means an actual forward paper order against the current market.

A paper EXIT means an actual forward paper exit.

Use actual paper fills/order lifecycle.

Do not turn the historical excursion detector into a fake execution engine.

Do not assume requested quantity == filled quantity.

Do not use historical B/C boundaries as paper fills.

---

# 27. Forward learning has two separate evidence streams

During PAPER stage, distinguish:

## A. Market outcome evidence

The same delayed tape-fragment detector continues observing the real-time market.

It eventually tells us what fragment actually occurred:

```text
UP / DOWN / CHOP / FLAT
```

and where causal B/C boundaries resolved.

This allows forward scoring of the model's live precursor predictions.

## B. Paper execution evidence

The paper broker tells us:

- what orders were actually placed;
- what actually filled;
- entry/exit fill economics;
- fees;
- realized paper P&L.

These two evidence streams answer different questions.

Do not merge them prematurely.

---

# 28. Forward learning may refine the same model

There is one predictive model.

Do not create:

```text
historicalModel
paperModel
liveModel
```

as parallel learners.

Once a forward outcome is resolved:

1. score the frozen live prediction first;
2. score the actual paper result where applicable;
3. only then allow that newly resolved example to update the same cognition model.

This is online/prequential learning.

The paper outcome may refine the model only after it has been used as genuinely unseen forward evidence.

---

# 29. Do not destroy the proof by immediately training on it

Every forward example has two moments:

```text
before learning outcome:
    evidence of genuine forward skill

after scoring:
    eligible new training evidence
```

The dashboard and statistics must preserve that distinction.

Do not count an observation as forward skill after the model has already learned that outcome.

---

# 30. Stage D — Forward skill demonstrated

Historical skill proves the model can learn from stored chronological tape.

It does not prove it survives the current real-time market.

Forward paper evidence is the stronger test.

Track forward-only measurements independently from historical measurements.

At minimum:

## Forward recognition

- ENTER predictions;
- correct ENTER opportunities;
- false ENTER by DOWN/CHOP/FLAT class;
- misses;
- timing error to B;
- EXIT predictions;
- premature exits;
- correct exits;
- misses;
- timing error to C.

## Forward paper economics

- completed paper round trips;
- realized paper return;
- return squared;
- mean paper return;
- standard error / lower bound;
- fees;
- fill/slippage information already available from the paper venue.

Do not mix historical returns into these statistics.

---

# 31. Forward skill does not mean live-money authority

This task ends at proving paper-forward skill.

Do NOT automatically enable real-money trading.

A future change may decide whether the forward paper evidence is strong enough to authorize real execution.

For this task:

```text
historical learning
    →
historical skill
    →
forward paper learning
    →
forward paper skill
```

is sufficient.

Real-money authority remains explicitly separate.

---

# 32. Training dashboard is part of the implementation

The existing frontend Training dashboard must become the truthful observability surface for this entire process.

This is not optional.

Do not create a second learning dashboard.

Do not add a separate “new model” page.

Improve the existing training dashboard.

Every displayed number must come from real backend state.

No decorative placeholder charts.

No hardcoded progress.

No fabricated normal distributions.

No zero used to represent unavailable evidence.

---

# 33. The dashboard must show the current training stage

The training dashboard must prominently show the canonical backend-owned stage.

For example:

```text
MODEL DEVELOPMENT
HISTORICAL VALIDATION
FORWARD PAPER LEARNING
FORWARD SKILL DEMONSTRATED
```

Do not infer this in React.

Training owns it.

The UI renders it.

Also show why progression is blocked when a stage gate is not yet met.

Examples:

```text
waiting for valid exit evidence
historical return uncertainty still spans zero
no forward paper round trips completed
model has no support for current precursor
```

These reasons must be actual backend facts.

---

# 34. Visualize Iceberg replay progress

During model development, the dashboard should make it obvious that the model is learning from stored tape, not trading.

Show real values such as:

- run/epoch currently replaying;
- replay tick / boundary;
- completed fragments processed;
- fragments by UP/DOWN/CHOP/FLAT;
- unsupported fragments;
- model associations learned;
- trie size/support;
- current replay symbol;
- current A/B/C markers where applicable.

If total progress is genuinely knowable from the Iceberg run, show it.

If it is not knowable, do not invent a percentage.

---

# 35. Visualize the Impulse Map learning

Continue using the existing Impulse Map visualization.

It should display the real map being used for that training/replay context.

Useful truthful information includes:

- cell positions;
- active/inactive state;
- basin membership;
- region strength;
- region authority;
- region orientation/condition;
- topology lock/unlock state;
- currently selected active regions;
- meaningful temporal changes as the replay moves.

Metric names may be available on hover/inspection as provenance.

They must not become semantic labels the model consumes.

The visualization should help a human see anonymous observations clustering and regions developing.

---

# 36. Visualize the temporal precursor

This is essential.

The current dashboard should show the actual temporal precursor fragment being presented to the trie.

Do not merely display the current region list.

Show a compact sequence:

```text
I₀ → I₁ → I₂ → I₃ → …
```

Each moment should identify its region-condition tokens without pretending to know their human meaning.

For a supervised historical fragment, visually mark:

```text
A     B                 C
|-----|-----------------|
precursor   held excursion
```

The timeline must make clear which observations were available before each boundary.

---

# 37. Visualize UP / DOWN / CHOP / FLAT outcomes

The training dashboard must show the actual completed fragment class.

Maintain real counts and recent examples for:

```text
UP
DOWN
CHOP
FLAT
unsupported
```

Do not color everything in terms of “good” and “bad.”

These are tape classifications.

Economic usability is a separate property.

For an UP fragment, separately show whether B→C cleared friction.

---

# 38. Visualize what the trie believed BEFORE training

For each resolved historical or forward fragment, show the frozen pre-outcome prediction where practical:

```text
context
predicted action
support
confidence
ambiguity
actual delayed label
correct / incorrect
```

This must be the prediction captured before the outcome trained the trie.

Do not show the post-training answer and call it the prediction.

---

# 39. Trie visualization must remain real

Continue using the real radix trie visualization.

It should show real:

- branch/context identities;
- support/count;
- action association;
- policy bias / probability evidence;
- ambiguity where available.

Do not invent “edge” from class probability.

Do not decorate a branch with economic P&L unless economic statistics are actually stored for that branch.

If no economic statistic exists at that level, do not display one.

---

# 40. Historical skill panel

The dashboard must contain a clearly labeled historical held-out section.

Show separately:

## Entry recognition

```text
UP opportunities
correct ENTER
missed ENTER
false ENTER on DOWN
false ENTER on CHOP
false ENTER on FLAT
```

## Exit recognition

```text
continuation WAIT correct
premature EXIT
correct EXIT
missed EXIT
```

## Historical economics

```text
held-out predicted entries
mean executable return
return uncertainty / SE
lower bound
```

If undefined, render undefined / insufficient evidence.

Not zero.

---

# 41. Forward paper panel

Once the system enters forward paper learning, the same dashboard should visibly switch/add a distinct forward section.

It must show:

```text
LIVE MARKET
PAPER EXECUTION
FORWARD / UNSEEN
```

Useful information includes:

- current focused symbol;
- current precursor sequence;
- current trie prediction;
- whether the model was authorized to paper ENTER/EXIT;
- active paper position state;
- paper entry fill;
- current mark;
- unrealized paper P&L;
- exit fill;
- realized paper P&L;
- unresolved forward predictions awaiting outcome;
- resolved forward predictions;
- forward recognition statistics;
- forward economic statistics.

Never mix replay trades and paper trades in the same number.

---

# 42. Make historical and forward evidence visually unmistakable

A human looking at the dashboard must immediately be able to tell whether a displayed event is:

```text
HISTORICAL REPLAY
```

or:

```text
LIVE FORWARD PAPER
```

Do not reuse “Agent ENTER” markers for historical counterfactual boundaries.

Use truthful names.

For example:

Historical:

```text
OPPORTUNITY B
CAUSAL EXIT C
REPLAY PREDICTION
```

Forward:

```text
PAPER ENTER
PAPER ENTRY FILL
PAPER EXIT
PAPER EXIT FILL
```

Do not visually imply execution where there was none.

---

# 43. The existing forward tape visualization must become genuinely forward

If the Training dashboard has a “Forward Learning” visualization, it must represent actual forward paper observations once Stage C begins.

Before Stage C, it may display historical validation only if clearly labeled historical.

Never call replay data forward.

Forward means:

> the model made the prediction before the future market outcome existed.

---

# 44. No duplicate frontend state

Follow the current subtractive frontend architecture.

Do not restore deleted aliases such as old Atom/Store names.

Do not create a special second store for the same training state.

One backend fact has one frontend owner.

Derived presentation values are derived from that fact.

Do not publish the same quantity through unrelated feeds merely because an old component expects it.

---

# 45. Filter before transport

Use the existing focus symbol and active route to avoid sending irrelevant high-volume data.

The Training dashboard normally needs:

- its current focus symbol;
- aggregate training stage/statistics;
- limited model/trie summaries;
- the current relevant replay or forward fragment.

Do not send every full Impulse Map for every symbol to the browser if the frontend only displays one.

All-symbol aggregate counts may be sent when the UI actually displays an aggregate.

Apply focus filtering before encoding wherever possible.

---

# 46. Training telemetry should describe training only

Do not smuggle portfolio state into generic training metrics if the frontend already receives canonical portfolio/paper-position state from its real owner.

Training should own:

- stage;
- model development progress;
- fragment supervision;
- predictions;
- held-out recognition statistics;
- historical skill;
- forward scoring statistics;
- skill-gate status.

Paper positions belong to the paper execution owner.

The dashboard may combine them visually.

The backend should not duplicate ownership to make that convenient.

---

# 47. Persist only what is necessary

Iceberg is the durable market/history source.

The predictive model checkpoint may persist model state and enough exact training progress to resume without double-learning.

Do not persist duplicate copies of the market tape inside the model.

Do not persist frontend presentation state.

Forward paper outcomes should become durable evidence through the appropriate existing persistence path if one already exists.

Do not invent a second database for training.

---

# 48. Model development and paper learning use one model

There is one trie.

Historical replay develops it.

Forward paper outcomes refine it after held-out scoring.

Do not fork the model when entering the paper stage.

The model's logical revision must remain observable so we can tell which model revision issued each forward prediction.

---

# 49. Tests required — historical learning

Add or update real tests proving:

### Temporal precursor identity

Two identical final Impulses reached through different temporal developments can produce distinct temporal contexts.

### Snapshot regression

Prove the learner is no longer merely classifying the final B snapshot.

### UP

Proper A→B prefixes → WAIT.

Complete profitable A→B → ENTER.

Holding B→C prefixes → WAIT.

C → EXIT.

### DOWN

Entry context → WAIT.

### CHOP

Real material non-directional tape → WAIT.

### FLAT

Real quiet valid tape → WAIT.

Missing tape != FLAT.

### Friction

Gross-positive but net-negative UP → WAIT.

### Peak leakage

Profitable extremum but non-profitable causal C must not produce profitable training return.

### Exit causality

C must be the detectable end, not retrospective optimum.

### Replay/live identity

Same input boundary produces identical precursor context.

### No frequency weighting

Repeated unchanged Impulse state does not multiply evidence.

### No magic precursor cap

A naturally longer fragment is not truncated to a fixed token count.

---

# 50. Tests required — staged training

Prove:

1. a fresh model begins in historical development;
2. replay alone cannot create paper trades;
3. held-out predictions are scored before learning;
4. insufficient historical skill cannot enter paper-learning stage;
5. defined historical skill can transition to paper-learning stage;
6. stage is owned by backend Training state;
7. frontend renders the real stage and blocker;
8. historical and forward counters remain separate.

Do not bypass the gate in tests by directly setting a “ready” boolean.

Drive the actual evidence required.

---

# 51. Tests required — forward paper learning

Use a deterministic paper/live test harness around the real ownership boundaries.

Prove:

1. the live model emits a prediction before the outcome;
2. an authorized ENTER creates a real paper order;
3. requested order != assumed fill;
4. actual paper fills determine position state;
5. unresolved prediction does not train the outcome;
6. after the market fragment resolves, the frozen prediction is scored;
7. only after scoring may the resolved forward example train the trie;
8. actual paper realized return is recorded separately from historical opportunity return;
9. forward skill statistics contain no historical samples;
10. a losing paper trade remains a losing observation;
11. false ENTER on DOWN/CHOP/FLAT is counted;
12. premature and missed EXIT are counted.

---

# 52. Tests required — frontend

The existing Training dashboard must have tests showing that it renders actual backend data for:

- training stage;
- replay progress;
- UP/DOWN/CHOP/FLAT counts;
- current temporal precursor;
- A/B/C boundaries;
- frozen predicted action;
- actual delayed label;
- trie support;
- historical held-out metrics;
- paper position state;
- forward paper P&L;
- forward-only skill metrics.

Tests must fail if the backend field is absent.

Do not satisfy them with hardcoded component defaults.

Also prove:

- unavailable measurement renders as unavailable, not `0`;
- replay ENTER is not rendered as PAPER ENTER;
- paper fill is not rendered before a fill exists;
- historical and forward sample counts cannot be accidentally summed.

---

# 53. Remove obsolete code while doing this

The completed implementation should leave exactly one learning path.

Delete directly superseded code including, where applicable:

- instantaneous precursor-only learning;
- prefix-of-one-snapshot training used as fake temporal history;
- peak-profit-as-return logic;
- artificial ENTER/+return and WAIT/-return opposition if replaced by proper categorical supervision;
- fake historical agent entry/exit telemetry;
- obsolete training dashboard code;
- duplicate historical/forward counters;
- stale comments describing the old learner;
- fixed region-count logic;
- fixed precursor-order caps;
- symbol fallback behaviour.

Do not preserve them “for compatibility.”

Git is the source compatibility history.

Persisted-data compatibility is a separate matter and must be explicitly justified.

---

# 54. Do not optimize for profitability during implementation

Do not tune the detector to make historical curves look better.

Do not alter thresholds after looking at profit.

Do not select fragment definitions because they maximize P&L.

Do not tune against the same tape being reported as validation.

The purpose is to create a falsifiable system.

---

# 55. Completion criteria

This work is complete only when all of the following are true:

- the Impulse Map produces empirically selected regions;
- temporal precursor keys represent actual sequences through time;
- UP/DOWN/CHOP/FLAT all come from genuine tape;
- historical A/B/C boundaries are causal;
- peak hindsight does not leak into realized training economics;
- WAIT receives real negative examples;
- ENTER and EXIT are trained from proper temporal prefixes;
- held-out historical predictions are scored before training;
- historical skill is measured separately from model-fit statistics;
- paper trading cannot begin before historical skill is demonstrated;
- paper trades occur genuinely forward against the real-time market;
- forward predictions are frozen before outcomes;
- forward paper outcomes are scored before being used for additional learning;
- historical and forward evidence remain separate;
- the same trie is progressively refined;
- real-money authority is NOT automatically enabled;
- the existing Training dashboard truthfully visualizes all stages;
- the frontend contains no fake training values;
- route/focus filtering happens before unnecessary transport;
- no parallel old learning path remains;
- `gofmt` passes;
- Go tests pass;
- race tests pass;
- frontend typecheck/lint/tests pass.

---

# 56. Required final report

Do not end with “implemented successfully.”

Report the experiment precisely.

Include:

### Representation

- What one Impulse state contains.
- What one temporal precursor contains.
- What constitutes a meaningful temporal transition.
- What A, B and C mean.

### Fragment detection

- How UP is detected.
- How DOWN is detected.
- How CHOP is detected.
- How FLAT is detected.
- What measured quantities determine boundaries.
- What previously arbitrary constants were removed.

### Supervision

- Exactly what teaches WAIT.
- Exactly what teaches ENTER.
- Exactly what teaches EXIT.
- How negative examples enter the trie.
- How unchanged states avoid frequency weighting.

### Leakage protection

- Why no observation after B can influence the A→B key.
- Why no observation after C changes the C target.
- Why the price extremum cannot become a fake exit.
- Why replay and live use identical representation.

### Historical development

Report real counts:

```text
fragments:
  UP:
  DOWN:
  CHOP:
  FLAT:
  unsupported:

entry held-out:
  correct:
  false:
  missed:

exit held-out:
  correct:
  premature:
  missed:

economics:
  predicted entries:
  mean executable return:
  standard error:
  lower bound:
```

Do not omit bad results.

### Stage gate

Report:

- current stage;
- whether historical paper-entry authority is available;
- the exact measured reason.

### Forward paper evidence

If enough forward time/data exists, report:

```text
paper entries:
paper completed trades:
paper realized return:
paper mean return:
paper return uncertainty:
false enters by fragment class:
entry timing error:
exit timing error:
premature exits:
missed exits:
```

If forward evidence does not yet exist, say so.

Do not substitute historical evidence.

### Frontend

List exactly which Training dashboard elements are wired to:

- model-development state;
- Iceberg replay;
- fragment classification;
- precursor sequence;
- trie state;
- historical validation;
- paper position/fills;
- forward validation.

Confirm that no displayed value is synthetic.

### Deletions

List the obsolete implementations, aliases, fallbacks, fake metrics and duplicate paths removed by the cut.

---

# Final rule

Do not make the system look like it works.

Make the system capable of telling us whether it works.

The desired progression is:

```text
REAL HISTORICAL MARKET TAPE
        ↓
IMPULSE MAP DEVELOPMENT
        ↓
TEMPORAL FRAGMENT LEARNING
        ↓
PREQUENTIAL HISTORICAL SKILL
        ↓
PAPER AUTHORITY
        ↓
REAL-TIME FORWARD PAPER TRADING
        ↓
FROZEN-PREDICTION FORWARD SCORING
        ↓
ADDITIONAL ONLINE LEARNING
        ↓
MEASURED FORWARD SKILL OR MEASURED FAILURE
```

Every arrow must represent something that actually happened.

No fakery.
No hidden future information.
No second model.
No architecture theatre.
No invented success.

---

I think the important addition here is that the two learning phases are deliberately asymmetrical.

Iceberg gives the model enough experience to become a plausible candidate without risking anything. But that can never prove the model works forward—it can only prove that chronological replay produces held-out skill on the recorded distribution.

Paper trading then becomes the actual examination.

The model says ENTER **now**, before it knows the future. Kraken paper has to actually fill it. It says WAIT or EXIT as the tape develops. Then eventually the market provides enough later evidence for the fragment detector to say what actually happened. Only at that point do we score the frozen decision, inspect the actual paper economics, and allow the observation to teach the trie.

That gives you a very clean hierarchy of evidence:

```text
model fit
    < historical prequential skill
    < real-time forward recognition skill
    < real-time paper execution skill
```

And the training dashboard should effectively tell that story from left to right. That would make it genuinely useful for answering the question you've been trying to answer all along: not “is the model learning something?”, but **“has it progressed from remembering historical structure to predicting unseen market structure before it happens, and can those predictions survive actual execution friction?”**