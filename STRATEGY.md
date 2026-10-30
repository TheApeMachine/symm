# Strategy flow (enter → hold → exit)

This is how live ticks become trades, paths, and learning updates.

## Pipeline per tick

1. **Signals** write kernel layers (`pumpdump`, `correlation`, `depthflow`, `hawkes`, `liquidity`, …).
2. **`mergeAggregateLayers`** fans board oracles onto every symbol on the tick.
3. **`Stage`** runs the grid (`Observe`) → 15 region brightness values per symbol.
4. **`Sequencer`** (Sequitur + shared MDL) turns regions into path letters:
   - **Enter** (flat): regime R06–R11 × posture **R00–R02** → `enter` grammar.
   - **Hold** (open lot): same regimes × posture **R06–R08** → `hold` grammar.
   - **Exit** (open lot, parallel step): regimes × posture **R03–R05** → `exit` grammar.
   - **R12–R14** veto spelling (risk oracle); they never become letters.
5. **`Model.Evaluate`** collapses Sequitur tails to path keys and queries the **path trie** (`model/paths/`):
   - `…/enter.bin` — open a lot (paper vs virtual from impulse vs resistance + gate).
   - `…/hold.bin` — reinforce staying in the hold sentence (trained on virtual settle).
   - `…/exit.bin` — cognitive profit exit (gate must allow; underwater exits are stop-owned).
6. **`Gate`** learns three hurdles (`ignition`, `risk`, `rejection`) from virtual round trips; each step publishes a **`gate`** frame on `training/step/{symbol}`.
7. **`broker.Desk`** applies MCTS **exit policy** at entry (`teachPolicy`); **`StopLoss`** runs invalidation, evaporation, trail, expiry on every mark.
8. On **virtual or paper settle**, `completeExit` updates enter/hold/exit path resistance and gate hurdles.

## Path keys on a position

| Field | Grammar | Trie suffix |
|-------|---------|-------------|
| `entry_path` | enter | `enter.bin` |
| `hold_path` | hold (R06–R08) | `hold.bin` |
| `exit_path` | exit (R03–R05) | `exit.bin` |

## UI (real time)

- **`training/step/{symbol}`** — lit regions, Sequitur events, `gate` vectors, `policy`, paths.
- **`positions/*`** — marks, `stop_state`, learned cut/trail/bars, PnL trace.
- Forward Learning **Open positions** — stop proximity, exit policy, **Live cognition** panel.
- Longtail — letter gate rates, path book, exit ledger (backend `training/longtail/aggregate`).

See also `SIGNALS.md` for metric ↔ region mapping.
