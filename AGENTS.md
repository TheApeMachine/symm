# AGENTS.md

## 🚨 CRITICAL SCOPE LOCK & CIRCUIT BREAKER

1. **STRICT SCOPE BOUNDARY:** Touch ONLY the files explicitly requested or strictly required to solve the task. Do NOT perform unprompted cleanups, renames, or refactorings in neighboring files. Architectural changes that are strictly required to preserve clear ownership/composition are part of solving the task; never use the scope lock as an excuse to pile unrelated behavior onto an existing receiver.

2. **THE "STOP WORK" RULE:** Once the requested task is solved and tests pass, **STOP IMMEDIATELY**. Do not look for "extra work", do not sharpen unrelated logic, and do not reorganize project architecture unless explicitly ordered to do so in the prompt.

3. **ZERO UNOWNED ABSTRACTIONS:** If a task is genuinely one cohesive behavior and can be solved in 10 lines of flat Go, write 10 lines of flat Go. But "flat" does **not** mean "put every new method on the nearest large receiver". Create a small type when the behavior has its own state, lifecycle, invariants, vocabulary, or reason to change. Every abstraction must own something real; speculative wrappers, empty interfaces, and namespace-only types are prohibited.

---

## Project Objective

**Maximize the wallet. Minimize the time to do so.**

A best-effort, highly principled market system. Detect real opportunity types—pumps, coils, exhaustion, liquidity vacuums, sector lifts, thin-book traps—and act on them with dynamically derived thresholds only.

* **No shortcuts or magic numbers:** No static time horizons, hardcoded multipliers (`*2`), or un-statistic denominators.

* **No fakery or performative math:** Implement real, solid, rigorous signal math using honest market data streams.

---

## Anti-Patterns (Zero Tolerance)

- **Fixed Time Windows:** Hardcoded windows (e.g. 60s) copied from external repos.
- **Ticker Summaries as Microstructure:** Scoring aggregate summary fields and calling it microstructure.
- **Positive-Only Returns for Dump Detection:** Exhaustion requires lift decline AND price rejection context.
- **Single-Spike Test Fixtures:** Tests must use multi-leg replay via the `tests/market` system.
- **Merged Category Masses:** e.g., combining `trendMass + flatMass` under a single wire key.
- **Bare Multipliers:** Expressions like `*2` or `(1-x)` without a proper statistical denominator.

---

## Code Style & Architecture (Go)

### Composition & Simplicity**

* **Composition Means Ownership:** A type should own one cohesive concept: its state, lifecycle, invariants, and operations. If a behavior can be named independently and has a reason to change independently, prefer a small composed owner for it.

```go
type Position struct {
    StopLoss *StopLoss
}

type StopLoss struct {
    // Stop-loss state belongs here.
}
```

`Position` coordinates/owns `StopLoss`; stop-loss behavior belongs to `StopLoss`. Do not put stop-loss methods on `Position` merely because `Position` already exists.

* **God Receivers Are Prohibited:** Splitting one giant receiver across `foo.go`, `foo_bar.go`, `foo_baz.go`, etc. is **not composition**. It is one god object with a file-system namespace. If unrelated method families accumulate on the same receiver, extract real owners and compose them.
* **Root Types Wire; Owners Work:** High-level types such as `Agent`, `Solver`, `Execution`, `BookManager`, etc. should primarily wire composed owners and coordinate lifecycle. They must not become dumping grounds for every behavior in their subsystem.
* **State Lives With the Behavior That Owns It:** Do not keep a field on a parent receiver while implementing all of its behavior elsewhere through helper functions. Move the state and the methods together into the owning type.
* **One Owner for Shared Behavior:** If the same domain behavior, calculation, construction rule, protocol setup, normalization, fee/price logic, or other invariant is needed in multiple places, give it one canonical owner and make every caller use that owner. Do not copy the behavior into each consumer. This rule applies equally to production code and tests. `price.go` being the only place that owns price/fee behavior is one example of the principle; the rule is broader than price.
* **Change Amplification Is an Architecture Smell:** A local implementation detail should not force mechanical edits across dozens of unrelated files. If changing a constructor such as `kraken/websocket/conn.go` means updating 126 call sites, the construction boundary is wrong. Fix the fan-out by introducing or repairing the appropriate composition root, constructor owner, fixture, builder, or other single responsibility boundary. Do not spend hours propagating the same mechanical change through consumers when one owner can absorb it.
* **Construction Has an Owner Too:** Shared dependencies should be assembled in one appropriate place. Callers should request/use the composed dependency rather than each independently knowing every concrete constructor argument. Do not hide meaningful domain choices, but centralize incidental wiring and defaults so constructor churn stops at the composition boundary. Prefer a concrete constructor owner or small builder over speculative interfaces.
* **Tests Must Reuse Shared Test Infrastructure:** Do not let every test invent its own mock API, fake websocket, fixture graph, or dependency constructor. Repeated test setup is shared behavior and needs one canonical test owner. Build reusable fixtures/builders/mocks in the appropriate test-support boundary and let individual tests specify only what is semantically different for that case. A production constructor signature change should normally require updating the shared fixture once, not every test file that consumes it. Test duplication is architectural duplication.
* **File Size Is a Design Signal, Not a Permission Slip:** Around **200 LOC** in one implementation file is suspicious and should trigger an ownership/composition review. Do not wait for 400+ lines before considering composition, and do not mechanically split files just to satisfy a number. The question is whether the file/type still represents one cohesive concept.
* **Receiver Sprawl Is a Design Smell:** Before adding another method to a large receiver, ask whether the method operates on that receiver's core invariant or whether it belongs to a composed owner. If the only reason is "this is the object I already have", that is not sufficient ownership.
* **Zero Proxy/Passthrough Methods:** Never write a wrapper method on a struct that merely calls a method on an embedded field or another object. Expose embedded types or call them directly. Composition should reduce receiver surface area, not recreate it through forwarding methods.
* **Flatter Is Better Inside an Owner, Not Across the Entire Architecture:** If a method exists solely to pass data through a 1-line chain, delete it and call the real owner directly. This does **not** mean flattening several independent responsibilities onto one receiver.
* **Methods Over Loose Functions:** Group logic into cohesive domain types. Keep implementations direct and simple. Use package-level functions for genuinely stateless transformations; do not use loose helpers to disguise behavior that should have an owner.
* **Interfaces Must Describe a Real Boundary:** Do not introduce an interface merely to make the architecture look abstract. Use one when there are genuinely multiple implementations, a meaningful substitution boundary, or an existing protocol that requires it. Prefer concrete composition otherwise.
* **No Pseudo-Namespaces:** A filename, method prefix, or comment section is not an architectural boundary. `capitalFoo`, `capitalBar`, and `capitalBaz` methods on the same unrelated receiver are not a `Capital` component. Make the component real.
* **Delete the Old Path When Replacing Ownership:** When behavior moves into a composed owner, remove obsolete fields, methods, helpers, and compatibility shims from the previous owner. Do not leave two paths that can drift apart.

### Control Flow & Safety

* **Early Returns & Guard Clauses:** Keep primary logic at indentation level 1.
* **No `else` Blocks:** Invert conditions and return/exit early.
* **Nesting Ceiling:** Max 2 levels of `if` nesting. If logic requires deeper branching, simplify the state checks instead of scattering code into helper methods.
* **No Silent Failures:** Return descriptive errors. Substituting default fallbacks or ignoring errors is prohibited. Let unexpected panics surface rather than hiding them. However, returning an error is often still silent, so use the `errnie.Error` method to wrap your returned error. `errnie.Error` is entirely transparent and will return your error, also when nil, but will log the error when not nil at the same time.

```go
func (someType *SomeType) SomeMethod() error {
    return errnie.Error(errors.New("some error description"))
}
```

* **No checks for NaN or Inf** Prefer to let the system crash, because this will give us the clearest signal possible that somewhere there is something wrong. By handling these cases and letting the system continue, we have silent death in the system. If for example we return 0 as the fallback and then use that in a multiplication, we instantly zero out the operations. Just let the system crash, then we can solve the cause and not the effect.

**A NOTE ON PERFORMANCE**

This hot path is quite extreme, we're dealing with over 640 symbol pairs in the universe, each one pushing the full Level3 stream into the system. Because of this, the system has been designed to be a **streaming iterator**. This means:

1. Ideally we do not: allocate, accumulate, copy, clone, snapshot, etc.
2. Every Step method gets one chance to observe the incoming value, perform its processing step, and return its output value.
3. It is essential that everything **always** returns an output value when called upon. It is understood that there legitimately can be baselines, windows, etc. that would normally require a "warmup" period, but this should be handled differently in this system. First of all, all baselines, windows, or other temporal structures should be dynamic and adaptive in the first place. For example, if you need a baseline, you must make the first incoming value the "current" baseline, and keep growing the span of the baseline for as much as is needed to result in a true, stable and sharp baseline, which needs to be calculated. This **should** be handled by `nomagique`'s `baseline` implementation. If at any given moment the baseline loses its sharpness/stability, the span should either grow or shrink dynamically until it is stable again. This will allow you to always output a value when `Step` is called, and fields like `maturity` or `snr` will indicate to sub-systems downstream how much to rely on any given value.

4. When it comes to processing steps that genuinely need history, consider the following:

```go
// If you would normally do something like this.
accumulator := 0.0

for _, bid := range book.Bids {
    accumulator += bid.Price
}
```

It is basically just a matter of removing the `for` and seeing the `Step` method itself as one iteration of the `for` loop.

The above is just an example of course. Also, given that most computation should be using `nomagique.Number` pipelines, there are already stateful features built into `nomagique` which you should be using versus keeping state on the type itself.

> !NOTE
> One of the more important takeaways here is that you should protect the throughput at all times. If you notice any issues, even if not related to your current tasks, you should at the very least mention it, so we can decide directly what to do about them.

### Formatting & Naming

* **No Single-Character Variable Names:** Variable receivers and variables must be descriptive (e.g., `signalCalculator`, not `s`). Exception: `t *testing.T` and `b *testing.B`.
* **Line Formatting:** `if` statements MUST have an empty newline above them (unless at the very start of a block). Wrap long parameter lists cleanly across line breaks.
* **Errors & Logging:** Error variables must be named `err`. Log errors strictly via `errnie`:

```go
errnie.Error(errnie.Err(
    errnie.Validation,
    "[package] descriptive error message",
    err,
))
```

* **Test Structure Mirrors Code Structure** Only have test files that mirror the code files (<codefile>_test.go) and only have test methods that mirror the methods in the code file that is being tested (func <CodeMethod>Test(t *testing.T)). Benchmarks follow the same function naming rules and are always at the bottom of the test file. Always use Goconvey for tests, and use BDD-style nesting for your test cases. Treat tests code as you would implementation code, and keep things clean and DRY.

## GoConvey Best Practices Example

```go
package main

import (
    "database/sql"
    "testing"
    _ "github.com/lib/pq"
    . "github.com/smartystreets/goconvey/convey"
)

func WithTransaction(db *sql.DB, f func(tx *sql.Tx)) func() {
    return func() {
        tx, err := db.Begin()
        So(err, ShouldBeNil)
        Reset(func() {
            _, err := tx.Exec("SELECT 1")
            So(err, ShouldBeNil)
            tx.Rollback()
        })

        f(tx)
    }
}

func TestUsers(t *testing.T) {
    db, err := sql.Open("postgres", "postgres://localhost?sslmode=disable")

    if err != nil {
        panic(err)
    }

    Convey("Given a user in the database", t, WithTransaction(db, func(tx *sql.Tx) {
        _, err := tx.Exec(`INSERT INTO "Users" ("id", "name") VALUES (1, 'Test User')`)
        So(err, ShouldBeNil)

        Convey("Attempting to retrieve the user should return the user", func() {
             var name string
             data := tx.QueryRow(`SELECT "name" FROM "Users" WHERE "id" = 1`)

            err = data.Scan(&name)
            So(err, ShouldBeNil)

            So(name, ShouldEqual, "Test User")
        })
    }))
}

Convey("Setup", func() {
    foo := &Bar{}

    Convey("This creates a new variable foo in this scope", func() {
        foo := &Bar{}
    }

    Convey("This assigns a new value to the previous declared foo", func() {
        foo = &Bar{}
    }
}

Convey("Top-level", t, func() {
    db.Open()
    db.Initialize()

    Convey("Test a query", t, func() {
        db.Query()
    })

    Convey("Test inserts", t, func() {
        db.Insert()
    })

    Reset(func() {
        db.Close()
    })
})
```

## IMPORTANT

PLEASE DO NOT TRY TO SOLVE EVERY ISSUE BY LOOKING AT GIT HISTORY.
THERE IS A REASON IT IS CALLED HISTORY, AND IT IS RARELY THE WAY
FORWARD, EVEN IF TECHNICALLY AN OLDER SOLUTION WOULD WORK AND "SOLVE"
AN ISSUE, IT WAS OBVIOUSLY REJECTED FOR A REASON.

`/**/` style comments only for top-level structures, inline comments use `//`.

```go

/*
Sample represents a fully populated market ticker payload point.*
*/

type Sample struct {
    Symbol        string  `json:"symbol"`
    AggressorSide string  `json:"-"`
    Bid           float64 `json:"bid"`
    BidQty        float64 `json:"bid_qty"`
    Ask           float64 `json:"ask"`
    AskQty        float64 `json:"ask_qty"`
    Last          float64 `json:"last"`
    Volume        float64 `json:"volume"`

    // StepVolume is the quantity executed in this step alone, as opposed to
    // Volume, which is the cumulative traded quantity the ticker reports.
    StepVolume float64   `json:"step_volume"`
    VWAP       float64   `json:"vwap"`
    Low        float64   `json:"low"`
    High       float64   `json:"high"`
    Change     float64   `json:"change"`
    ChangePct  float64   `json:"change_pct"`
    Timestamp  time.Time `json:"timestamp"`
}

```

## FINALLY*

The user is looking to you for help and/or solutions. Just because you have a "human-in-the-loop" tool, doesn't mean you should use it.

The answer to almost every question you feel like asking, in most of the cases is the one that results in the most principled solution, and is often **not** the easiest, simplest, or quickest way.

That being said, taking the effort to do the work once is still infinitely easier, simpler, and quicker, than trying to reward-hack it, and then having to do everything again, and again, until it is correct. So you may as well do it right the first time.

Also, "pre-existing" really just means "bugs I left laying around in some previous session" so yes, you do own them, and yes, you should fix them if you encounter them. Otherwise they will never get fixed.

> !NOTE
> Things in this code-base might change, and very often some old implementation code, or legacy is left orphaned, and is not properly cleaned up. There are two things to consider here. First, you should not do this, if you move something around, always clean up any left over code that should no longer be there. This is to keep things maintainable, but also to avoid confusion later. Second, if you notice that there is some old, orphaned code, **never ever** decide you should add some weird backwards compatability shim, or hook into that in any way. Just clean up the old code, and implement things according to what is very obviously the new, latest, most recent path.
> As an extension of the above, if you are replacing some current functionality **never ever** implement a secondary system, while leaving the old system around. Always prefer replacing the existing system outright, and reusing any existing structure where possible, versus introducing new structure.