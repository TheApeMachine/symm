# RULES

- Underscores are **NOT** namespaces, and just splitting things out into separate files is **NOT** proper composition.
- For a proper example of composition, look at how price.go and instrument.go compose onto desk.go
- Create small objects, using only methods, not functions, and compose functionality together. **NO GOD OBJECTS**
- Tests **MUST** use Goconvey and **MUST** mirror the file names they are testing with `_test.go`.
- Before you call a task done, first pull the files you changed, and re-read them in full again.

## Config

- `system.Cfg` is constructed with validated defaults in the `system` package. **Never** re-validate it.
- Do **not** write `if system.Cfg != nil`, `if system.Cfg.Risk != nil`, `if x > 0 { use config } else { hardcode }`, or helper fallbacks that duplicate defaults.
- Read values directly: `system.Cfg.Risk.MaxAllowedLossPct`, `system.Cfg.Learning.MinEnterZ`, etc.

## Methods and nil

- Do **not** add methods whose only job is to return a field or forward to another method (`MaxLoss()`, `Floor()`, `ActiveFloor()` wrappers, `Done()` that only returns `.done`, etc.). Use the field, or call the real collaborator.
- Do **not** nil-guard the method receiver (`if x == nil { return }`). Constructors always build a usable instance; calling a method on nil is a bug, not a branch.
- Do **not** nil-guard composed collaborators the constructor always sets (`if stop.budget == nil`, `if learning.stop == nil`). Fix the constructor.
- Nil checks on **external** inputs (args, map lookups, channel payloads, optional wiring like a missing transport) are fine. Receiver/collaborator ceremony is not.

## Layout and readability

- Match the vertical rhythm in `strategy/symbols.go`: **blank lines separate logical steps**, not every line.
- After guard clauses (`if nil { return }`), leave one blank line before the body of the method.
- After setup (parse key, decode layer, bind locals), leave one blank line before the next decision or loop.
- Inside loops, put a blank line between **prepare** (parts, metrics, symbol) and **act** (append, merge, continue).
- Put a blank line between separate phases (e.g. per-pair layers, then aggregate fan-out), and before the final return.
- Prefer one small type per concern (e.g. `Symbols` on the tick) with a few methods; do not grow packages of loose helpers.
- Do not use `map[string]struct{}` set idioms when a slice plus `slices.Sort` / `slices.Compact` is enough for tick-scale data.
- Keep comments short (type or method purpose in one line). Avoid essay blocks and redundant doc on constructors.
