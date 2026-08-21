# `signal/pumpdump` — volume-clocked ignition evidence

PumpDump measures whether executed volume and directional price movement are
becoming unusual for one symbol, whether the executable spread is coiling, and
whether an active move is exhausting. It emits measurements only. Category
selection and trading decisions remain downstream.

## Input and clock

Each executed trade is paired with the most recent ticker touch at or before
the trade timestamp. The signal passes the trade quantity, trade price, bid,
ask, timestamp, and the Frame's fixed retention capacity into
`nomagique/algo.Ignition`.

Ignition uses a volume clock. A bar closes when accumulated executed quantity
reaches the median positive trade quantity retained for that symbol and event
time has advanced beyond the bar opening time. This makes the bar size adapt to
each market's own tape instead of imposing a shared wall-clock window or volume
threshold.

## Causal baselines

The algorithm retains bounded histories for trade quantities, completed-bar
rates, absolute completed-bar returns, and executable spreads. Every current
reading is compared only with history that existed before that reading:

- `rvol` is completed-bar volume rate divided by the prior median bar rate.
- Buy and sell `precursor` are the positive and negative parts of the current
  log return divided by the prior positive-return median.
- `compression` is `max(0, 1 - spread / prior median spread)`.
- Buy and sell `exhaustion` combine a decline from the prior RVOL reading with
  opposite-direction price rejection against the retained return scale.

The positive precursor baseline is derived from the return history in place.
It does not retain a duplicate precursor ring. The spread history occupies the
fourth fixed history family, so compression adds no per-symbol history bank.

## Measurement contract

Every trade with an executable touch emits one `pumpdump` measurement carrying:

| Metric | Meaning |
|---|---|
| `hypothesis_separation` | Normalized margin between the competing buy and sell exhaustion hypotheses. |
| `best_price:buy`, `best_price:sell` | Executable bid and ask used for this observation. |
| `midpoint` | Midpoint of that executable touch. |
| `trade_price`, `trade_quantity` | Raw print used to advance the volume clock. |
| `rvol` | Current completed-bar rate relative to its causal rate baseline. |
| `spread` | Raw ask-minus-bid; its normalized value is spread divided by midpoint. |
| `compression` | Current spread tightening relative to the prior spread median. |
| `precursor:buy`, `precursor:sell` | Reciprocal directional price-move evidence. |
| `exhaustion:buy`, `exhaustion:sell` | Reciprocal directional exhaustion evidence. |

`Raw` is always present. Competing normalized evidence is withheld until the
rate, return, and spread histories required by the classifier exist. The spread
fraction remains available immediately because it is defined entirely by the
current executable touch.

Every measurement also sets:

- `At` to the trade event time.
- `ObservedFrom` to the opening event time of the completed volume bar that
  produced the current score. Before a completed bar exists it equals `At`.
- `Horizon` to `At - ObservedFrom`.
- `Maturity` to `completedBars / (completedBars + 1)`.
- numeric metadata for capacity, raw trade and touch values, event-time
  coordinates, completed-bar rate, rate baseline, and spread baseline.

Hypothesis separation is zero when neither exhaustion hypothesis has evidence,
one when only one is supported, and otherwise the winner's normalized margin
over its competitor. It is populated on every measurement and becomes
classified normalized evidence with the rest of the directional output.

## Downstream meaning

The category stage combines these measurements with the other signal
perspectives. In particular:

- elevated RVOL plus a directional precursor can support vertical ignition;
- spread compression can support a coiled-compression opportunity;
- sustained precursor evidence without a volume spike can support organic
  trend;
- declining RVOL plus rejection can support faded exhaustion.

PumpDump does not select any of those stories itself. It preserves the measured
evidence and its provenance so category, graph, and planner can reason over it.
