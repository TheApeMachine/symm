# `logic/manifold` — the market as a fluid

> One gas. Every symbol breathes into it.

## What this package is

Most trading systems treat an order book as a table: price levels, sizes, a
spread. This one treats it as **matter**. Every resting order becomes a particle
with mass, position, and frequency; the particles are injected into a running
quantum-hydrodynamics simulation; and the market's structure is read back out of
the *field* those particles produce rather than out of the book they came from.

The physics is not a metaphor layered on top of the numbers. It is the actual
computation: a Metal GPU kernel integrating a coupled gas + omega-wave system,
living in [`nomagique/physics/fluid`](../../../nomagique/physics/fluid). This
package is the market-facing half — it decides *what becomes a particle*, feeds
the domain, and reads the result back as something a trading system can use.

**The single most important structural fact:** there is exactly one `Domain` for
the entire symbol universe. BTC and some illiquid altcoin are not independent
simulations. They inject into the same gas, and they interfere. That is
deliberate — cross-symbol interference is signal, not contamination — and it
shapes nearly every design decision below.

## The pipeline

```
order book
    │
    │  Tokenizer.NewBatch      (token.go)
    ▼
particles  ────────────────►  Domain.Append  ──►  Domain.Advance
 mass/ω/φ                     (shared gas, all symbols)      │
    │                                                        │
    │                                    ┌───────────────────┤
    │                                    ▼                   ▼
    │                              Domain.Reading      Domain.Display
    │                              (thesis.Manifold)   (GPU image → UI)
    │                                                        
    └──►  Domain.SourceDial  ──►  geometry.Corpus  ──►  phase sweep
          (per-symbol ω fingerprint)   (retained history)   (phase.go)
```

## Tokenization — turning orders into matter

`token.go` is where the market becomes physics, and every field of a particle is
a deliberate claim about what an order *is*.

| Particle field | What the order contributes                                                                                                            |
|----------------|---------------------------------------------------------------------------------------------------------------------------------------|
| **Mass**       | Observation multiplicity. Each injected order contributes one unit; inelastic collision adds units into persistent carriers.         |
| **Omega (ω)**  | Log distance from mid, normalized by the symbol's own accumulated scale. This is the particle's **identity** — its content frequency. |
| **Phase (φ)**  | Queue rank by exchange price-time priority. Sequence position.                                                                        |
| **Position**   | Where it lands in the 64³ spatial grid.                                                                                               |

Three subtleties worth understanding, because each one was a bug first:

**ω is signed and centered on mid.** Bids sit below the lattice centre, asks
above it. An earlier version normalized by the book's *extent*, which handed a
lopsided book the entire lattice and let its bids land in ask territory.

**ω uses `tanh`, not a hard clamp.** Extent normalization pinned the nearest and
furthest order to exactly `OmegaMin`/`OmegaMax` every tick, making them
spuriously resonant with each other. `tanh` is monotone and bounded — distance
ordering survives, and nothing can rail. Its tail compression is a consequence of
that choice, not a market law.

**Mass begins at the observation unit.** An ordinary resting order enters with
mass `1`, independently of its exchange lot units. Order quantity is already
encoded by the Y position. Repeated observations acquire mass only when the
Metal inelastic merge combines matching carriers, preserving the original
tokenizer invariant that mass measures additive observation multiplicity.

**φ comes from queue rank, not slice index.** The caller assembles order slices
by walking a Go map, whose iteration order is randomized per tick. Using the
slice index re-randomized every resting order's phase on every batch, destroying
exactly the relative phase offsets that carry positional information.

## Folding — compression as evidence

`fold.go` meters the inelastic merge inside `Advance`. The thesis is that
*collision is compression*: repeated observations of the same content identity
should collide and accumulate into single heavy particles. That is how the
manifold turns a stream of ticks into mass.

The meter exists because a manifold that never folds is indistinguishable from
one that folds correctly unless you measure it, and the two failure modes are
opposite:

- **Fold rate ≈ 0** — content identities never collide. The population is a
  stream of strangers and mass never accumulates.
- **Fold rate ≈ 1** — identities collide too easily. Distinct observations are
  being summed into each other.
- **Eviction** — neither. The residency cap discarding oldest particles by
  policy, which suppresses resident count for reasons unrelated to compression
  and would otherwise be misread as folding.

Excitation state is tracked with the same care: a Hawkes fit that has not
converged is *unmeasured*, not zero, and the two must not be conflated.

## The phase dial

`phase.go` implements a market port of the **Sensorium HCAM phase-rotation
experiments** — the same mechanic as the Fashion-MNIST phase-steered morphing and
the semantic geodesic scan over a text corpus.

**The original experiment.** Encode items into complex fingerprints. Rotate a
query fingerprint's global phase by α ∈ [0, 2π). At each angle, measure signed
overlap against every stored item. What you get is not noise — it's a structured
hand-off of resonance from one stored concept to another, tracing a geodesic
through the manifold. Rotating "Democracy requires individual sacrifice" by 180°
lands on "Nature does not hurry, yet everything is accomplished": the categorical
antipode of active political sacrifice turns out to be passive natural emergence.

**The market port.** The stored items are past market states. The label is what
price then did.

1. **Fingerprint.** `Domain.SourceDial` bins one symbol's injected particles onto
   the ω lattice (complex sum of `mass·e^{iφ}`), then multiplies elementwise by
   the resident wave mode ψ. The multiply is what makes it a *reading of the
   field* rather than a restatement of the order book — a symbol only lights up a
   mode it both occupies and the field is actually excited at.

2. **Retain.** Each cut's dial waits `phaseOutcomeHorizon` (8) manifold advances,
   then is stored in a `geometry.Corpus` tagged with the realized forward log
   return over that span, classified `up`/`down`/`flat`.

3. **Sweep.** `Corpus.ScanPhasesExcluding` evaluates 72 angles, top-K 16. The
   overlaps are computed once per entry and rotated analytically —
   `real(overlap · e^{-iα})` — so angle count costs one complex multiply per
   entry, not a re-encode.

### Two decisions that carry the design

**The label is ground truth, never a model's opinion.** An earlier version tagged
the corpus with the cognition stage's DMT classification. That is wrong twice
over: cognition is a Markov context over a radix trie, an entirely different
substrate that has no business reaching into a hydrodynamics kernel; and tagging
history with a classifier's output makes the scan report how self-consistent that
classifier is rather than whether the field has structure. Price is the only
ground truth the manifold can read directly, and it needs no other subsystem.

**The `flat` dead zone is derived, not chosen.** Classifying by sign alone labels
every cut by its last tick of noise. A fixed threshold is equally wrong — the
same fractional move is decisive for one symbol and inside the spread for
another. The tokenizer already accumulates each symbol's RMS log distance of
resting orders from mid, which is that book's own measure of how far price must
travel to mean anything. `flat` means "did not clear its own book scale."

Two smaller ones, both anti-footguns:

- **The current cut is excluded from its own sweep** by timestamp. Otherwise the
  query selects itself at every constructive phase, pinning the dial at α=0 with
  a perfect response that means nothing.
- **Attribution via `ReadSpatialIDs` was rejected.** Those IDs keep only the low
  eight bits of the content token, truncating the symbol index to seven bits and
  silently aliasing symbols together once the universe passes 128. symm runs
  ~800.

### Reading the dial

Sweeps are stamped on `Thesis.Phase` per symbol as a `types.PhaseReading`, and
mirrored onto the wire row for the UI. `PhaseReading.Alignment()` returns the
most constructive angle — where the ray points — and what that history did.

Warm-up is real: 8 cuts to mature one entry, 32 entries before a sweep publishes
as ready. Until then the reading carries `Reason: "retaining history"`. That is
the readiness gate working, not a stall.

**Nothing consumes `Thesis.Phase` yet.** The data is stamped and rendered; no
gate, rule, or allocator reads it. A trade today comes out identical with the
dial deleted. That is a deliberate seam, not an oversight.

## Files

| File           | Responsibility                                                 |
|----------------|----------------------------------------------------------------|
| `solver.go`    | Owns the shared `Domain`. Appends, advances, reads, publishes. |
| `token.go`     | Order book → particles. The market/physics boundary.           |
| `phase.go`     | Fingerprint retention, outcome labelling, angular sweep.       |
| `fold.go`      | Compression and excitation metering.                           |
| `binary.go`    | SMF1 packing of GPU display frames for the UI.                 |
| `constants.go` | Admissibility bounds.                                          |

## Gotchas

- **Metal `.metal` edits need `go run ./metallibgen`**, not just `go build` —
  the shader is `go:embed`ed.
- **Near-vacuum cells can NaN.** Global pre-step substep sizing cannot see a cell
  that thins mid-step; the fix is CFL-0.8 substep targeting plus a kernel that
  freezes a violating cell rather than producing NaN.
- **Heat needs a floor.** `Heat = Amplitude·CV` pinned sound speed at √15 and
  zeroed all Hawkes forcing; `Heat = 0` NaN'd the GPU outright. A CV floor of
  1/32 plus rest-frame velocity variance is the fix.
