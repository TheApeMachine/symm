# `logic/cognition` — the market as a sentence

> Every other stage asks *what is happening now*.
> This one asks *what usually happens next, given how we got here*.

## What this package is

Cognition is the only stage in the chain with a **memory of sequence**. Category
produces a regime verdict each tick and forgets it. Resonance settles a latent
state and moves on. Cognition takes the stream of category verdicts and treats it
as a **language**: each regime is a token, a run of regimes is a sentence, and the
question becomes what any language model asks — given this prefix, what comes
next, and how surprised should I be by what actually arrived?

The substrate is a **radix trie** (`dmt.Tree` in
[`datura/dmt`](../../../datura/dmt)) holding a Markov context model. This is worth
stating plainly because it is easy to mistake for the other subsystems: it shares
*no machinery* with the manifold's quantum hydrodynamics or resonance's
predictive coding. Different data structure, different mathematics, different
failure modes. DMT is prefix statistics over discrete tokens.

## The vocabulary

Each tick, per symbol, the dominant category becomes one token:

```
aggressive_drive → book_thinning → frenzy → exhaustion
└──────────── one sequence, up to maxSeqLen (6) ─────┘
```

`dmt` tokenizes on `_`, so that is the sequence separator. But category names
*contain* underscores (`aggressive_drive`), which would split one regime into two
tokens and corrupt every prefix in the trie. So `encodeCategory` swaps the
underscores inside a category name for `\x1f` (ASCII unit separator) before
joining, and `decodeCategoryToken` swaps them back on the way out:

```
aggressive_drive  →  aggressive\x1fdrive        (encode)
aggressive\x1fdrive_book\x1fthinning            (joined sequence)
```

The trie therefore sees exactly one boundary per regime transition, and `\x1f`
never appears in a category name so the mapping is lossless.

**Only transitions are recorded.** If the dominant category is unchanged from
last tick, nothing is appended. A regime that persists for a thousand ticks is one
token, not a thousand — otherwise dwell time would drown out structure, and the
model would learn that markets mostly repeat themselves, which is true and
useless.

## Three namespaces in one trie

The tree partitions its key space by prefix, which is what lets one structure hold
three different kinds of memory:

| Namespace         | Key shape                 | Holds                                                                 |
|-------------------|---------------------------|-----------------------------------------------------------------------|
| `s/` **sensory**  | `s/[sequence]`            | Prefix counts and conditional probabilities — the Markov model itself |
| `e/` **episodic** | `e/[timestamp][sequence]` | Completed sequences awaiting replay                                   |
| `b/` **basin**    | `b/[class]/[sequence]`    | Attractor basin posteriors — which macro regime a sequence belongs to |

Values are a packed 16-byte `{Count uint64, Probability float64}`.

## Sequence breaks: surprisal as punctuation

The central mechanic. A sequence ends when the market stops making sense:

```go
if len(activeTokens) >= maxSeqLen        { break }   // 6 — hard ceiling
if surprisal > surprisalLimit            { break }   // 3.5 bits ≈ P < 8.8%
```

Surprisal is `−log₂ P(token | prefix)`. Above 3.5 bits, the transition that just
arrived was one the model considered under ~9% likely — and that is exactly the
definition of a **regime break**. The sentence is over; a new one starts with the
surprising token as its first word.

This is the elegant part: the model does not need a rule for what a regime change
looks like. A regime change *is* the point where its own predictions stop working.

On a break, three things happen in order:

1. **Commit to episodic buffer** — the completed sequence is timestamped and
   stored under `e/` for later replay.
2. **Unsupervised learn** — attractor basin weights are strengthened for the
   sequence that just completed.
3. **Start fresh** — the buffer resets to the single new token.

## What is computed each tick

| Step      | Call                                  | Produces                                                             |
|-----------|---------------------------------------|----------------------------------------------------------------------|
| Classify  | `Classify`                            | Attractor basin scores — which macro regime this sequence belongs to |
| Lookahead | `ExecuteBeamSearch` (width 3, 2 hops) | Probable next category paths                                         |
| Ambiguity | `MeasureBranchAmbiguity`              | Shannon entropy of the branch vs. a uniform-split baseline           |
| Contrast  | `ComputeBasinContrastiveEvidence`     | KL divergence between the top two competing classes                  |

**Contrast and confidence answer different questions.** Confidence is the winner's
score. Contrast is the gap to the runner-up. A 90%-confident winner with an
89%-confident runner-up is a coin flip wearing a confident hat, and `Ambiguous`
exists to say so — entropy measured against what a uniform split *would* look
like, so the threshold adapts to branch count rather than being a fixed number.

The `Branches` output walks every prefix of the active sequence with its count and
probability, so the whole decision path is inspectable rather than just its
verdict.

## REM sleep

Every 128 ticks, over a 60-second window:

```go
tree.ExecuteREMSleepConsolidation(startWindow, nowUnix)
```

This is a deliberate analogue of memory consolidation during sleep, and it does
what the biology is thought to do:

1. **Replay** — walk episodic entries in the window and retrain sensory weights
   from them.
2. **Re-optimize** — rerun classification weight optimization on each replayed
   sequence.
3. **Retroactive inhibition** — decay the entire sensory namespace, *preserving*
   the prefix paths that were just replayed.

Step 3 is what makes it consolidation rather than mere repetition. Everything
fades; only what was replayed is protected. Patterns that stopped recurring lose
their grip, and the model tracks the market that exists now instead of
accumulating every regime it has ever seen. The decay factor is derived from the
replay count against namespace size — not a constant.

## Ordering

Cognition runs **after** category and consumes `thesis.Categories`. It is gated on
`thesis.Readiness.Categories`: no verdicts, no tokens, no pass.

Its output then feeds `logic/graph`, where the beam-search predictions become
`leads` / `lags` edge pairs — the current winner regime `leads` each predicted
category, weighted by path probability.

## Files

| File        | Responsibility                                                              |
|-------------|-----------------------------------------------------------------------------|
| `solver.go` | Tokenization, sequence breaks, classification, beam search, REM scheduling. |

Upstream in `datura/dmt`:

| File                          | Responsibility                                             |
|-------------------------------|------------------------------------------------------------|
| `cognitive.go`                | Packed weights, surprisal, context weight access.          |
| `cognitive_schema.go`         | The three namespaces and their key layouts.                |
| `cognitive_engine.go`         | Sensory training, basins, beam search, REM consolidation.  |
| `cognitive_reasoning.go`      | Contrastive evidence, entropy, analogy, decay.             |
| `cognitive_classification.go` | Basin classification and unsupervised weight optimization. |

## Gotchas

- **Scratch buffers are reused.** `classScratch` and `beamScratch` live on the
  solver to keep the hot path allocation-free. They are not safe to share across
  goroutines — cognition processes symbols sequentially, unlike resonance and
  causal.
- **The WAL rotation race.** A "dmt/tree" error flood is a datura WAL-rotation
  concurrency bug — snapshot held only `walMu`, not the tree's `persistMu` — which
  latches a fatal. It surfaced only once the manifold Heat/CV fix let cognition
  write the WAL at full rate. Fixed via a `saveSnapshotLocked` split, but worth
  knowing the shape of.
- **Cognition is not ground truth.** It is a classifier over its own prefix
  statistics. Tagging another subsystem's retained history with its output makes
  that subsystem measure DMT's self-consistency instead of whatever it was built
  to measure. (The manifold's phase corpus made this mistake and now labels with
  realized price instead.)
