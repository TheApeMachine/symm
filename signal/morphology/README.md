# Morphology / Order-Book Shape Signal Specification

## 1. Purpose

The Morphology signal measures the *shape* of an order book as a geometric object: where displayed notional sits along the price axis, and how that shape changes over time.

It answers:

1. How far apart, on average, are the bid and ask depth shapes?
2. What is the worst local cumulative disagreement between the two shapes?
3. How concentrated is each side's displayed depth?
4. How disordered (spread-out) is each side's displayed depth?
5. How much did the whole book's shape move since the previous observation?

The signal does **not** classify a book as manipulated, spoofed, synthetic, suspicious, thin, toxic, or anything else. It measures geometry and reports numbers.

---

## 2. First Principles

### 2.1 Shape coordinates

Let the best bid price be `b` and the best ask price be `a`, with

```text
spread  = a - b        > 0
mid     = (a + b) / 2
```

For an aggregated price level at price `p` with displayed quantity `q`, its
**position** is its distance from the midpoint in spread units:

\[
\boxed{
r = \frac{p - \text{mid}}{\text{spread}}
}
\]

so the bid touch sits at `r = −0.5` and the ask touch at `r = +0.5`. Its
**weight** is its displayed quote notional:

\[
\boxed{
w = p\,q
}
\]

A side's *shape* is the probability mass over positions obtained by
normalizing its level weights to a unit sum. Price is normalized by the current
spread; weight is normalized by the side's own notional. The result is
dimensionless and unitless — a shape, not a quote of size or price.

### 2.2 Bilateral distance

Two shapes over the same sorted position support are compared with two
complementary measures of distribution distance:

**Wasserstein-1 (earth mover's distance)** — the integral of the absolute
difference of their cumulative masses:

\[
\boxed{
W_1(P,Q)=\int\left|F_P(r)-F_Q(r)\right|\,dr
}
\]

It answers *"on average, how far apart are the two depth shapes?"*, measured in
spread units.

**Kolmogorov–Smirnov statistic** — the supremum of the absolute cumulative
difference:

\[
\boxed{
D(P,Q)=\sup_r\left|F_P(r)-F_Q(r)\right|
}
\]

It answers *"what is the worst cumulative local disagreement?"* and is
dimensionless in `[0,1]`.

The two shapes live on different price supports in general, so they are first
placed onto their **sorted union of positions**, each side contributing zero
mass at a position it does not occupy. This is the exact comparison of two
empirical distributions, with no resampling and no invented grid.

### 2.3 Concentration and entropy

For one side's normalized weights `w_i`:

**Concentration (Herfindahl)**:

\[
\boxed{
H=\sum_i w_i^2
}
\]

in `(0,1]`; equals `1/n` for uniform depth over `n` levels and `1` for a single
monopolized level.

**Entropy (Shannon, natural units)**:

\[
\boxed{
S=-\sum_i w_i\ln w_i
}
\]

in `[0,\ln n]`; zero for a single level, `ln n` for uniform depth.

Concentration measures dominance; entropy measures disorder. Both are shape
facts, emitted per side, dimensionless (entropy in nats).

### 2.4 Structural change

The whole-book shape (both sides folded into one mass profile, each side halved
so the total remains unit) is retained per symbol. **Structural change** is the
Wasserstein-1 distance between the current whole-book shape and the previously
retained one, on their shared support:

\[
\boxed{
\Delta_t = W_1(\text{shape}_{t-1},\text{shape}_t)
}
\]

It is undefined on a symbol's first observation (no prior shape exists) and is
never fabricated as zero.

---

## 3. Measurement Envelope

Every measurement carries `From`, `At`, `Maturity`, and `SNR` per the global
envelope. Because morphology is a stateless direct measurement of the current
book shape (the retained prior shape is one overwritten entry, not a fitted
estimator), `Maturity` is `1` and `SNR` is undefined (no noise model applies to
a distribution distance).

---

## 4. Metric Set

| Metric | Unit | Meaning |
|---|---|---|
| `book_shape_distance` | dimensionless (spread units) | W₁ distance between normalized bid and ask depth shapes |
| `book_shape_ks` | dimensionless `[0,1]` | KS statistic between bid and ask depth CDFs |
| `concentration:bid` | dimensionless `(0,1]` | Herfindahl concentration of the bid shape |
| `concentration:ask` | dimensionless `(0,1]` | Herfindahl concentration of the ask shape |
| `entropy:bid` | nat | Shannon entropy of the bid shape |
| `entropy:ask` | nat | Shannon entropy of the ask shape |
| `morphology_change` | dimensionless (spread units) | W₁ distance from the previous whole-book shape |

---

## 5. Invalid and Missing States

The signal MUST distinguish:

1. no shared book for the symbol;
2. a crossed book (`spread ≤ 0`);
3. an empty bid or ask side;
4. a book whose levels carry no positive notional.

Rules:

- a book with no shape yields **no measurement** (the caller skips it), never a
  fabricated zero distance;
- `book_shape_*`, `concentration:*`, and `entropy:*` are undefined when either
  side is empty;
- `morphology_change` is undefined on a symbol's first observation.

---

## 6. Explicit Non-Claims

The Morphology signal does not determine:

- whether a book is spoofed, synthetic, manipulative, or toxic;
- whether an asymmetry is "suspicious";
- whether a shape presages a move;
- any fixed symmetry threshold or "normal" depth band.

It reports dimensionless book-shape geometry only. Interpretation — historical,
temporal, or cross-sectional — belongs downstream.
