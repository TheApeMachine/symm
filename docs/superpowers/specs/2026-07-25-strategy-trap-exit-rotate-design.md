# Strategy Trap / Exit / Rotate Design

Date: 2026-07-25

## Goal

Maximize wallet, minimize time. Detect real opportunities and refuse traps using dynamically derived evidence only — no magic thresholds, no silent fallbacks.

Sharpen three failure modes against controllable `tests.Market` tapes through the production boot graph:

1. False entries on absorption / low-volume lift / spoof
2. Late exits — retreat must freeze phantom adverse; sincere dumps must stop
3. Rotation — displace only when challenger edge clears incumbent exit cost

## Principles

- Forecasts that mint positive executable return on a trap tape are incorrect claims about the market. Fix them first.
- Strategy adds an explicit trap veto only where relative trap evidence still dominates after forecast pricing (e.g. spoof-inflated capacity).
- Trap vs opportunity comparison uses observed measurement masses already on Thesis: `trapShare = trapMass / (trapMass + opportunityMass)`. No static cutoffs.
- Proofs are absolute stage outcomes on known tapes, not calm < stress alone.

## Design

### A. Forecast honesty (`logic` adverse)

When minting a forecast, derive `trapShare` for the symbol from Thesis measurements:

- **Trap mass:** max normalized strength among toxicity spoof / retreating quantity, CVD absorption, liquidity starvation-class metrics present for the symbol.
- **Opportunity mass:** max normalized strength among pumpdump ignition/trend/strength and CVD drive for the symbol.

Price adverse selection as:

```
GM = InformedFlow × Spread
TrapTax = trapShare × max(0, ExpectedReturn)
ExpectedAdverseSelection = GM + TrapTax
```

When traps dominate (`trapShare → 1`), adverse covers the return claim and `ExecutableReturn` cannot clear fees/spread/impact. When opportunities dominate, only Glosten-Milgrom adverse remains.

### B. Strategy trap veto (`strategy/opportunity`)

After utility would otherwise clear, if `trapShare > 1 - trapShare` (traps strictly dominate opportunity mass), append `ActionNothing` with cause `trap_dominant` and a reason naming the dominant trap family. Challengers under Arbiter/Rotate inherit the same refuse — no enter, no displace-from-trap-symbol as challenger.

### C. Exit sincerity (proof + gap fill)

Existing Stoploss rule stands: retreat freezes geometry unless calibrated/causal forward return is negative.

Market-sim saga:

1. `FastPump` → enter + armed stop
2. `SpoofLiquidity` or `LiquidityRetreat` → hold full qty (no stop)
3. `FastDump` → full-lot `ActionExit` cause `stop`

Unit gap: prove negative calibrated return pierces retreat (if not already covered).

### D. Rotation (proof + Gate honesty)

Market-sim saga:

1. Fill normal slots on weaker pumps (isolate symbols)
2. Stronger `FastPump` on a free symbol while slots full
3. Assert displace: exit cause `rotation`, enter redeploy notional equals incumbent notional when Gate clears; otherwise explicit `rotate_wait`

No new rotate formula unless a proof fails for a principled reason (missing evidence in Gate inputs).

## Out of scope

- New signal packages
- Partial position reduces
- Reviving removed catalog Prove* harness names
- Static windows or universal trap thresholds

## Success criteria

| Tape / saga | Required outcome |
|-------------|------------------|
| VolumeAbsorption (alone) | no `ActionEnter` |
| LowVolumeLift (alone) | no `ActionEnter` |
| SpoofLiquidity (alone) | no `ActionEnter` |
| FastPump (control) | enter still possible |
| Pump → Spoof/Retreat → Dump | hold through phantom, exit on dump |
| Full slots + stronger challenger | rotate or explicit `rotate_wait` with audited advantage |
