# Numerical Stoploss + Decision v1

Date: 2026-07-17

## Goal

Ship a testable exit/admit path driven by logic-layer numbers (manifold,
resonance, causal, cognition), not named market regimes or P&L color.

## Contract

- **Stoploss** owns live exits: `stop` and `take_profit`.
- Inputs are a thin `Evidence` projection from Thesis logic outputs + mark path.
- `weight ∈ (0,1]` updates on resonance forecast epoch resolution (residual/σ).
- `lockedFloor` max-monotone; `trailDistance` widens/narrows from weight and σ.
- Live stop = `max(lockedFloor, mark * (1 - trailDistance))` in return space.
- Take-profit fires when peak return is approached and resonance forward path
  turns non-positive (or residual z-score blows out).
- **Decide** admits entries; cognition buy support remains the entry gate.
- Decompose admit state instead of a poetic lookahead predicate:
  - `OpportunityMargin = ExpectedReturn - Uncertainty`
  - `CognitiveLead = Cognition.Confidence - Manifold.CoherenceMag2`
- `AllocationClass: reserved` only when **both** margin and lead are positive
  (anticipatory edge: reasoning committed before the basin has settled).
- Positive margin with non-positive lead stays normal-lane SNR.
- `IsOpportunity` mirrors reserved class for desk slot policy.
- Reserved slots are a separate overflow lane: normal-full books still admit
  opportunity entries into reserved capacity; non-opportunity cannot.
- PostMortem later journals the same Evidence + ExitReason scalars.

## Out of v1

- Named position-kind enums as control surface
- Decide-primary utility exits competing with the stop
- Kelly sizing (stop distance exposed for next iteration)
- Confirmation-tick weight counters
