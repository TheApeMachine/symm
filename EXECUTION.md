# Execution model

## Principle

**One economic model, three clocks.**

| Clock | Role |
|-------|------|
| **Live / paper** | `trader` → `broker.Desk` → `kraken/paper` (or live client). Authoritative for what we trade. |
| **Optimizer replay** | Steps a measurement tape and must call the **same fill, preflight, sizing, and instrument rules** as the desk — not a parallel ledger with copied math. |
| **Integration harness** | Already runs the production stack on synthetic tape; the long-term tune target is to score candidates through this path. |

The replay ledger (`optimizer/replay`) remains as a **headless orchestrator** (positions, triggers, wallet, tape stepping). It must not reimplement slippage, gates, or sizing. Those live in `broker/` and `execution/`.

## Current layering

```
measurement tape tick
    → reasoning.EvaluateStateful (shared with live story)
    → broker.QuoteFromMeasurement
    → broker.PreflightGates          (entries only; same as Desk)
    → execution.EntryDeployFraction  (shared with trader)
    → broker.PrepareEntryOrder       (when instrument rules are wired)
    → broker.SlippageFill / StressedSlippageFill
    → wallet settle (replay)  |  paper.Balances (live)
```

## Migration status

| Concern | Live/paper | Replay (after unification) |
|---------|------------|----------------------------|
| Book walk / slippage | `broker.SlippageFill` | `broker.SlippageFill` via `QuoteFromMeasurement` |
| Preflight | `broker.PreflightGates` in `Desk.AddOrder` | Same before entry |
| Entry fraction | `execution.EntryDeployFraction` | Same |
| Instrument min qty/cost | `InstrumentRulesCache` | Loaded from Kraken `AssetPairs` at tune boot (`optimizer.tune.load_instrument_rules`) |
| Partial depth | Paper partial fill | Partial fill (not block + penalty) |
| P&L reporting | `Balances.RealizedPnL` | EUR realized + return fraction (not mislabeled `profit_loss`) |

Replay entries size coin quantity from the **actual fill price** (`slot / (fill × (1+fee))`), matching `trader` deploy math. `PrepareEntryOrder` may raise to Kraken minimums but only when wallet cash covers the raised notional. Entry-batch preemption closes victims at **their** last observed price, never at the preempting symbol's tick.

## Anti-patterns

- Copying `walkBookFill` into replay (drift within weeks).
- Optimizer scores that skip preflight or Kraken minimums.
- Reporting `realized/capital` as `profit_loss` without labeling it as a return multiple.
