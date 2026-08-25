# `logic/category` — what is the market *doing*?

> Aggressive drive with thinning depth is a breakout.
> Aggressive drive into loaded depth is absorption.
> Same drive. Opposite trades.

## What this package is

Category is the first stage in the logic chain, and it answers the most basic
question in the system: **what kind of market is this right now?**

A category is a *hypothesis* — `aggressive_drive`, `hidden_absorption`,
`coiled_compression`, `spoof_trap`, `book_thinning`, `laminar`, `turbulent` — and
`types.CategorySchemas` names the normalized metric that supplies each one.
The stage combines repeated positive readings of the same category and runs the
resulting category scores through nomagique's classifier.

This is deliberately the shallowest stage mathematically and one of the most
important structurally, because everything downstream inherits its framing.

## The tally

Each schema-selected metric contributes its positive normalized magnitude to
one category. Repeated readings for that category are combined with an evidence
geometric mean, so duplicating a row cannot turn publication frequency into
additive evidence. The classifier compares every declared category, including
categories with zero current evidence, and assigns one pseudocount to each when
computing evidence-share confidence.

Negative, absent, or not-yet-normalized readings do not become zero-valued
support: they do not enter the category evidence set. If every category is
empty, the solver emits the explicit none artifact.

## What a verdict carries

| Field        | Meaning                                                                                     |
|--------------|---------------------------------------------------------------------------------------------|
| `Confidence` | Category evidence share with one pseudocount per category                                   |
| `Strength`   | Geometric mean of the category's positive normalized readings                               |
| `Maturity`   | Minimum maturity of the contributing readings; absent when no supporting reading reports it |
| `Surprisal`  | `−log₂ P(category)`                                                                         |
| `Supporting` | Which observables argued for                                                                |

Three of these deserve attention:

**Confidence and Strength are different questions.** Confidence is the
category's share of the complete category competition, including the declared
pseudocount. Strength is the geometric mean of the positive readings that
support this category.

**Surprisal uses the same confidence the verdict reports.** Every returned
category computes `−log₂(confidence)`; winner and competitors no longer mix
evidence-share confidence with a separate softmax distribution.

## Where categories go

Categories become **hypothesis nodes** in the evidence graph (`logic/graph`),
which relates the other stages' conclusions to them with `supports` /
`contradicts` edges. That graph then gates entries — the `evidence_opposition`
cause can veto a trade outright. Trap tapes (`SpoofedPump`, `Vacuum`, `Coil`) in
`tests/conditions` exist to prove that veto fires.

Category types live in [`types/category.go`](../../types/category.go).

## Files

| File                | Responsibility                                                  |
|---------------------|-----------------------------------------------------------------|
| `solver.go`         | Collection, geometric evidence combination, and classification. |
| `types/category.go` | The metric → category schema.                                   |
