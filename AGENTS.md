# RULES

- Underscores are **NOT** namespaces, and just splitting things out into separate files is **NOT** proper composition.
- For a proper example of composition, look at how price.go and instrument.go compose onto desk.go
- Create small objects, using only methods, not functions, and compose functionality together. **NO GOD OBJECTS**
- Tests **MUST** use Goconvey and **MUST** mirror the file names they are testing with `_test.go`.
- Before you call a task done, first pull the files you changed, and re-read them in full again.

## Layout and readability

- Match the vertical rhythm in `strategy/symbols.go`: **blank lines separate logical steps**, not every line.
- After guard clauses (`if nil { return }`), leave one blank line before the body of the method.
- After setup (parse key, decode layer, bind locals), leave one blank line before the next decision or loop.
- Inside loops, put a blank line between **prepare** (parts, metrics, symbol) and **act** (append, merge, continue).
- Put a blank line between separate phases (e.g. per-pair layers, then aggregate fan-out), and before the final return.
- Prefer one small type per concern (e.g. `Symbols` on the tick) with a few methods; do not grow packages of loose helpers.
- Do not use `map[string]struct{}` set idioms when a slice plus `slices.Sort` / `slices.Compact` is enough for tick-scale data.
- Keep comments short (type or method purpose in one line). Avoid essay blocks and redundant doc on constructors.
