# `logic/graph` — where the stages argue

> Four independent models. They will not agree. The graph is where that
> disagreement becomes information instead of noise.

## What this package is

Graph is the **last** stage in the logic chain, and it is the only one that sees
everything. Category has hypothesized a regime, resonance has forecast a return,
causal has estimated an uplift, cognition has picked a basin — each from its own
substrate, each with no knowledge of the others.

This stage compiles all of it into a **directed knowledge graph**: typed nodes
for what each stage concluded, and typed edges for how those conclusions relate.
The result is handed to strategy, which uses it to gate entries.

The critical property: an edge is not a summary. It is a *relationship with a
stated reason*. When a trade is vetoed, the graph can say which two stages
disagreed and by how much.

## Nodes

| Kind        | Emitted from                                      | Example ID                     |
|-------------|---------------------------------------------------|--------------------------------|
| `category`  | Category verdicts                                 | `cat:BTC/USD:aggressive_drive` |
| `resonance` | Predictive coding forecast, latent state          | `res:BTC/USD:forecast`         |
| `causal`    | Pearl ladder uplift, do-expectation, intervention | `causal:BTC/USD:uplift`        |
| `cognition` | DMT winner regime, class confidence               | `cog:BTC/USD:winner_regime`    |

Each node carries `Value`, `Strength`, `Confidence`, and `At` — the last of which
is what makes staleness detectable.

## Edges — the vocabulary

```
supports            contradicts         conditions
leads               lags                redundant_with
independent_of      stale_relative_to   incomparable_with
```

This vocabulary is the package's real contribution. Most systems collapse
inter-model relationships into a single weighted average, which destroys exactly
the distinctions that matter:

- **`contradicts`** — two stages disagree in direction. Actionable: this is a
  reason *not* to trade, and it is why the `evidence_opposition` cause can veto
  an entry outright.
- **`redundant_with`** — two stages agree because they are looking at the same
  thing. Their agreement is *not* independent confirmation, and treating it as
  such double-counts one observation.
- **`independent_of`** — no relationship. Distinct from "we did not check."
- **`stale_relative_to`** — one node is older than the other by more than the
  stale threshold (default 5s). Agreement between a fresh node and a stale one is
  agreement with the past.
- **`incomparable_with`** — the relation is undefined, said out loud rather than
  quietly scored as zero.
- **`leads` / `lags`** — temporal precedence, emitted as a symmetric pair from
  cognition's beam-search lookahead: the current winner regime `leads` each
  predicted category, and that category `lags` it, both weighted by the path
  probability.
- **`conditions`** — one node sets the regime the other must be read in.

## The rule that shapes everything: compare direction, not magnitude

When relating a resonance forecast to a causal uplift:

```go
agreement, _ := agreementWeight(resonanceNode.Value, causalNode.Value)

if resonanceNode.Value > 0 && causalNode.Value > 0        →  supports
if signs differ                                            →  contradicts
```

`agreementWeight` is `min(|left|, |right|)` after magnitude weighting — **the
weaker of the two**, never a difference or a product of the raw values.

This is deliberate and load-bearing:

> The two heads score on unrelated scales. A raw magnitude comparison would let
> whichever head has larger units decide the relation by itself.

A forecast in log-return units and an uplift in ladder-score units are not
commensurable. Their *directions* are. And taking the minimum means a strong
claim paired with a weak one produces a weak edge — agreement is only as good as
its weakest participant.

Zero-confidence pair relations are not materialized at all, because their
decision weight is necessarily zero. An edge that cannot affect a decision is
noise in the graph.

## Edge confidence compounds

```go
Confidence: resonanceNode.Confidence * causalNode.Confidence
```

Multiplication, not averaging. Two 50%-confident nodes produce a 25%-confident
edge. This is the honest composition: a relationship inherits the uncertainty of
*both* endpoints, and averaging would let a confident node launder an uncertain
one.

## Category-centered structure

The graph is built around **category hypothesis nodes**. The other stages'
conclusions relate to them via `supports` / `contradicts`, and categories carry
their supporting, opposing, and *missing* evidence through from the category
stage.

That last one matters downstream: a hypothesis whose key evidence never arrived
is weaker than one whose evidence arrived and agreed, and the graph preserves the
difference rather than flattening both into "supported."

## Ordering

Graph runs **last** and depends on every stage before it:

```
category → manifold → resonance → causal → cognition → graph
```

The chain is sequential by design. Running it concurrently would mix
prior-epoch readiness with current-epoch values — a stage would read a neighbour's
output from the *previous* tick while believing it was current.

## Files

| File        | Responsibility                                                                  |
|-------------|---------------------------------------------------------------------------------|
| `solver.go` | Graph types, node extraction per stage, structural edge inference, publication. |

## Notes for extending

- **Adding a node kind** means deciding what it can *relate to*. A node with no
  edges is decoration.
- **Adding an edge type** means deciding what a consumer should *do* differently
  when it sees one. If the answer is "weight it slightly differently," it is
  probably a weight on an existing relation, not a new relation.
- **Every edge carries a `Reason` string.** Populate it. It is what turns a veto
  from "the system said no" into "the forecast and the causal uplift conflicted
  in direction."
