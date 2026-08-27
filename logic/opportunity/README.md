# Opportunity Synthesizer Specification

## 1. Purpose

The Opportunity Synthesizer converts discrete market category evidence into typed opportunity candidate hypotheses.

It tracks the lifecycle of asymmetric market transitions (pumps, coils, exhaustion, liquidity vacuums, sector lifts, thin-book traps) from early precursor agreement through ignition.

The Opportunity stage:
- **MUST NOT** re-derive raw measurements from market feeds;
- **MUST NOT** predict prices or bypass the Causal / MCTS decision stages;
- **MUST** track lifecycle phases causally per symbol and archetype family;
- **MUST** invalidate dissolved precursor states without lingering stale votes.

---

## 2. Opportunity Lifecycle Phases

```text
DORMANT ──(1 Precursor)──> FORMING ──(≥2 Precursors)──> ARMED ──(Confirmation)──> IGNITION
   ▲                          │                            │
   └───────────────(Dissolved / No Evidence)───────────────┴──> INVALIDATED (emitted once)
```

1. **`PhaseDormant`**: No precursor evidence active for this archetype.
2. **`PhaseForming`**: Exactly one precursor category active (e.g. `CoiledCompression`).
3. **`PhaseArmed`**: Multiple ($\ge 2$) distinct precursor systems agree (e.g. `CoiledCompression` + `HiddenAbsorption`).
4. **`PhaseIgnition`**: The confirmation category appears (e.g. `VerticalIgnition`), marking visible transition.
5. **`PhaseInvalidated`**: The precursor state dissolved without reaching ignition. Emitted exactly once to clear tracking.

---

## 3. Opportunity Archetype Families

| Archetype | Direction | Key Precursor Categories | Confirmation Category | Market Meaning |
| :--- | :--- | :--- | :--- | :--- |
| **`ArchetypeVerticalIgnition`** | `DirectionLong` | `CoiledCompression`, `HiddenAbsorption`, `BookThinning`, `Frenzy`, `AdverseLeverageBuildup`, `InefficientLag` | `VerticalIgnition` | Explosive volume and arrival cascade breakout. |

---

## 4. Downstream Interaction & Hindsight Integration

When an opportunity candidate is active, its archetype, phase, direction, sequence, and provenance are attached to the symbol's decision state.

The **Hindsight Workflow** reads `decision.Opportunity` and `decision.OpportunityType` to determine whether an missed trade was an **Opportunity Miss** (failure to recognize setup) or a **Policy Block** (filter, haircut, or margin blocker).
