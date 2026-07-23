# AGENTS.md

## Project objective

**Maximize the wallet. Minimize the time to do so.**

Miracles are not expected. A best-effort, highly principled system is. The goal is to detect as many real opportunity types as the market presents — pumps, coils, exhaustion, liquidity vacuums, sector lifts, thin-book traps — and to act on them with dynamically derived thresholds only.

Failure after an honest principled try is acceptable. Failure from magic numbers, incomplete data sources, or comment blocks that do not match implementation is not.

---

> real, solid, rigerous, and principled implementations of each signal using the extensive comment above the Signal type as the spec. Or refer to [DECISION.md](/Users/theapemachine/go/src/github.com/theapemachine/symm/DECISION.md) if the comment has been removed or altered.
>
> This also means:
>
> 1. No shortcuts, workarounds, and especially no fallbacks (unless highly defensible, usually not)
> 2. No magic numbers, no static math (time horizons, windows, etc.), and no assumptions that all symbols operate on the same temporal or any other scale.
> 3. Absolutely no fakery, performative math or implementation, or otherwise compromised mechanisms.
> 4. No "good enough" and no "lesser" implementation when a better one exists, which also includes being honest about what each signal needs to consume regarding market data, to make all of the above work.

**Once you complete a task, always go back and read over everything you created or changed once more, and see if you can refactor, or sharpen up the implementation, with a clear focus on maintainability and compactness.**

---

## Anti-patterns (learned the hard way)

- Fixed time windows (e.g. 60 seconds) copied from external repos.
- Scoring ticker summary fields and calling it "microstructure."
- Positive-only returns for dump detection — exhaustion needs lift decline **and** price rejection context.
- One-shot test fixtures (single spike) without multi-leg replay (use the tests/fixture system).
- Category masses merged invisibly (e.g. `trendMass + flatMass` with one wire key).
- Bare multipliers (`*2`, `(1-x)`) in classifiers without a statistic in the denominator.

## Definition of Done & Verification

Work is complete only when verified. You must provide proof of execution in your completion message.

* **Code Comments** Each type and each method must have a Godoc-style comment above it, explaining both the what and the why clearly, in a naturally flowing paragraph. For top-level comments like these we always use the `/**/` style, while for inline comments we use the `//` style.
* **Automated Tests:** Corresponding test coverage must exist, run, and pass for the exact code path changed.
* **Benchmarks:** A performance benchmark must exist and be executed for any data-processing or signal-calculation changes.
* **Verification Output:** You must paste the literal, unmodified stdout output of the test and benchmark runs in your response.

### Market-facing verification (simulation)

Market-facing behavior (signals, logic from market data, strategy decide/allocate/rotate/admit, order path) is proved through the **production boot path** with mock Conns as the only substitution:

1. **Transparent source:** `tests/mockapi` Conns → `websocket.NewAPI` → [`stack.Boot`](stack/boot.go) (same graph as [`cmd/root.go`](cmd/root.go)).
2. **Controllable tapes:** opportunity/trap catalog in [`tests/catalog`](tests/catalog) + multi-leg producers in [`tests/conditions`](tests/conditions) — pumps, coils, exhaustion, vacuums, sector lifts, thin-book traps, and adversarial twins.
3. **Drive:** `Crypto.Tick` / `Play` (Cut → Update → Plan → trade). Do not invent a parallel Planner harness. `CommitStrategy` is only for strategy/wallet truths **given** seeded forecasts, and must be labeled as such.
4. **Known outcomes:** assert stage truths (measure / decide / size / wallet), not merely “calm metric &lt; stressed metric.” Measure bounds may be `positive`, `present` (signed non-zero), or `zero` (e.g. subject has no lead claim).
5. **Use the catalog:** `tests/catalog` proves signals (`ProveMeasure` / `signals_test`), Analyzer side-effects on `Crypto.LastThesis` (`logic_test`), and strategy admit/size/rotate (`ProveStrategy` / `strategy_test`). Infrastructure without these proofs is incomplete.
6. **Signal packages own market proofs:** each `signal/*/signal_test.go` (or `market_test.go`) must boot `tests.NewSession` with that signal only, play `conditions.Tape*` streams, and assert absolute `tests.SourceClaim` outcomes (family peaks + stressed-vs-calm exceeds). Relative calm&lt;stress alone is insufficient. Catalog proofs do not replace package-local Session proofs.
7. **Exit honesty:** open-lot Regulate must be proved via catalog `ProveExit` (PlayOpen + adverse mark + CommitStrategy) — hold under phantom/shallow adverse with retreat, stop on sincere ungated breach, lock floor on calibrated lift. Unit Stoploss tests do not replace these Session proofs.
8. Pure unit math (decimal sizing scans, etc.) may remain; they do not replace Session/catalog proof for system behavior.

### Preventative Rules:

* **No Fabrication:** If tool or environment limitations prevent you from executing tests or benchmarks, state: `VERIFICATION LIMITATION: UNABLE TO RUN TESTS` and list the exact terminal commands you would run. Do not write mock or simulated test results.
* **Failing Tests:** If tests fail, you must stop and fix the code. Do not proceed or mark a task complete if any suite is failing.

---

## Code Style & Architecture

### Structure

* Prefer methods over functions. 
* Compose types to represent logical units.
* Do not use prefixes and underscores as artificial sub-groupings is package filenames. If there is a need to do so, it is a clear indication a sub-package needs to be created where those files can live without prefix and underscores.
* No artificial ceremony. Keep implementations simple, but effective. There is no need to constantly restructure things into more and more types, or to manually create what Go already has solutions for.

#### Go Structural Pattern

```go
package packagename

/*
ObjectName manages specialized domain logic.
It handles state updates for our trade calculations.
*/
type ObjectName struct {
    ctx    context.Context
    cancel context.CancelFunc
    err    error
}

/*
NewObjectName instantiates a new ObjectName with a canceled context.
*/
func NewObjectName(ctx context.Context) *ObjectName {
    ctx, cancel := context.WithCancel(ctx)

    return &ObjectName{
        ctx:    ctx,
        cancel: cancel,
    }
}

/*
MethodName performs a state operation.
*/
func (objectName *ObjectName) MethodName() {
    return
}
```

#### TypeScript Structural Pattern
* Use `const` arrow functions rather than standard function declarations.
* Use designated system flex, grid, and typography components instead of standard HTML equivalents.

```tsx
export const PaperEditorApp = () => {
	return (
		<PaperEditorProvider>
			<PaperContextSnapshot />

			<DragDropProvider>
				<Flex.Column className="box-border min-h-0 bg-background" fullHeight>
					<LatexToolbar />

					<Flex.Column className="min-h-0 flex-1" fullHeight>
						<WritingCanvas />
					</Flex.Column>
				</Flex.Column>
			</DragDropProvider>
		</PaperEditorProvider>
	);
};
```

> !IMPORTANT! The entire idea behind composition is so you hide away chunks of isolated complexity, which you can then consume, usually in 1 line of code. Don't hide away code, and then write a multi-line helper around that at the consuming site, then you're just bloating the code even further.

### Size Limits

* **File Size:** Target 200 lines; hard ceiling of 400 lines. Split files exceeding 400 lines into separate types/files.
* **Method Size:** Target under 30 lines. Methods exceeding 60 lines must be split into sub-methods, unless the operation is atomic (e.g., assembly kernels).
* **Type Size:** Limit types to a maximum of 10 methods.

> !IMPORTANT! This does *not* mean just move some methods to a new file and call it done. What this means is find the additional responsibilities that the object (type) is doing and compose those onto the current type as a new type. So take the example code above as the type that is over the line count, and do something like:

```go
/*
ObjectName is something descriptive.
It also has a reason why it was implemented.
*/
type ObjectName struct {
    ctx      context.Context
    cancel   context.CancelFunc
    err      error
    composed ComposedObject
}

/*
NewObjectName instantiates a new ObjectName.
It also has a reason for being instantiated.
*/
func NewObjectName(ctx context.Context) (*ObjectName, error) {
    ctx, cancel := ctx.WithCancel(ctx)

    obj := &ObjectName{
        ctx:      ctx,
        cancel:   cancel,
        composed: NewComposedObject(ctx)
    }

    return obj, errnie.Require(map[string]any {
        "ctx":    obj.ctx,
        "cancel": obj.cancel,
    })
}

/*
SomeMethod 
*/
func (object *ObjectName) SomeMethod() {
    // ... Do work ...
    results := object.composed.SomeMethod() // Do work using the composed object.
    // ... Do more work with results ...
}
```

> It is very important that you use composed objects and then encapsulate the logic in that object. Do not encapsulate and then add helper methods to call the methods in the encapsulation.

You should recognize objects that do too much when you have naming that is longer than two segments in either method names or object names.

```go
/*
MethodName.
*/
func (objectName *ObjectName) updateSomethingUnrelated() {
    return
}
```

Something like that is usually a good indicator that things are doing to much. In general you want to have one or two segments in names max. Above the ObjectName type is updating something that isn't itself.

```go
/*
MethodName.
*/
func (objectName *ObjectName) update() {
    return
}
```

Now ObjectName is clearly updating itself.

**Never use loose functions unless there is a very good reason for it.**
**Never use helper methods out of habit, they have to truly add value.**

### Control Flow

* **Early Returns:** Write guard clauses with early returns. Keep the primary logic path at indentation level 1.
* **Over Guarding:** Do not overly guard things, just let the system crash, at least we will know what goes wrong.
* **Over-use of `if` Statements** There are often much cleaner ways to set up branching control flow, especially when making proper use of composed types. We always prefer to use as little `if` statements as possible.
* **No Else Blocks:** Do not use `else`. Invert conditions to return early or exit.
* **Nesting Ceiling:** Do not nest `if` blocks deeper than two levels. Extract deeply nested logic into a helper method.
* **No Silent Failures:** If a precondition fails or an unexpected state occurs, return a descriptive error. Substituting default fallbacks or silently skipping errors is prohibited.

### Naming & Formatting

* **No Single-Character Names:** Variable names and method receivers must be descriptive (e.g., use `signalCalculator`, not `s`), the exception here is the `testing.T` and `testing.B` instance variable which should always be `t` and `b`.
* **Block Separation:** Insert an empty newline between distinct logical code blocks, except where there are only a few lines in a block or method/function. And `if` statements ALWAYS have an empty line above them, unless they are the first thing in a new block.
* **Line Breaks:** Wrap long function signatures to prevent lines from running past split-view boundaries. The two examples below are both correct ways to approach this, depending on how long the line is.

```go
myType.MyMethod(
    someParam, someOtherParam, oneMoreParam,
)

myType.MyMethod(
    someParam,
    someOtherParam,
    oneMoreParam,
    oneFinalParam,
)
```

What would be incorrect is something like this.

```go
thesis.Forecasts = append(thesis.Forecasts, types.Forecasts{
    Source: "resonance+causal", Symbol: state.Symbol,
    At: state.At, ObservedInterval: state.Duration,
})
```

Key/Value pairs should always be on their own line, so the formatting aligns everything in a nice, readable manner, like so:

```go
thesis.Forecasts = append(thesis.Forecasts, types.Forecasts{
    Source:           "resonance+causal", 
    Symbol:           state.Symbol,
    At:               state.At, 
    ObservedInterval: state.Duration,
})
```


* **Errors** Instance variables for errors are always `err` and nothing else. Errors are logged with `errnie`

```go
errnie.Error(errnie.Err(
    errnie.Validation, // Not the default, use the correct errnie.Kind
    "some message",    // or err.Error()
    err,               // or nil
))
```

## Testing

> !IMPORTANT! When it comes to tests, you should **always** use Goconvey, tests should mirror the code structure regarding filenames and methods, and use BDD style nested scenarios.

> Another thing about testing in this specific project is to always use the full market simulation system in ./tests/ to validate signals, decision making mechanisms, and anything else trading related, which is everything.

Follow the best-practices from Goconvey when it comes to decorators, setup, and teardown.

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
			/* Verify that the transaction is alive by executing a command */
			_, err := tx.Exec("SELECT 1")
			So(err, ShouldBeNil)

			tx.Rollback()
		})

		/* Here we invoke the actual test-closure and provide the transaction */
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

/* Required table to run the test:
CREATE TABLE "public"."Users" ( 
	"id" INTEGER NOT NULL UNIQUE, 
	"name" CHARACTER VARYING( 2044 ) NOT NULL
);
*/

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

    // setup (run before each `Convey` at this scope):
    db.Open()
    db.Initialize()

    Convey("Test a query", t, func() {
        db.Query()
        // TODO: assertions here
    })

    Convey("Test inserts", t, func() {
        db.Insert()
        // TODO: assertions here
    })

    Reset(func() {
        // This reset is run after each `Convey` at the same scope.
        db.Close()
    })

})
```

---

## Environment & Tooling Constraints

### Git State Integrity

* Do not read, query, or reference git history, commit logs, or previous branches to solve bugs. Base your solution entirely on the current state of the codebase. The answer/solution rarely lies in the past.
* Never run `git checkout`, `git reset`, `git restore`, or any command that discards working tree changes. If a revert is required, stop and request user intervention.

### Compiler Configuration & Linker Errors

* **dropg Linker Error:** If you encounter a `dropg` linker error, refer to the `Makefile` located in the project root to ensure environment flags and compiler options match the project targets. Do not bypass build constraints with temporary flags.

To fix this, your AGENTS.md file must act as an anti-bloat compiler. AI models default to deep nesting, defensive boilerplate, and proxy methods because they prioritize safety over ergonomics.
Create a file named AGENTS.md in your project root with the exact configuration below to hard-enforce flat architecture and proper composition.

---

## 🚨 Anti-Slop Directive: Core Philosophy

* Value Clarity Over Guarding: Do not write defensive boilerplate for impossible edge cases.
* Flatter is Better: If a function or method merely calls another function with no added logic, delete it.
* Ergonomics Win: Code must be brief, direct, and immediately readable.

### 🐹 Go Architectural Constraints## 1. Zero Proxy Methods & Strict Composition

* No Passthroughs: Never write wrapper/helper methods on a struct that simply call a method on an embedded or composed field.
* Expose Embedded Fields: Use Go's implicit embedding. Let the consumer call the nested method directly.
* No Artificial Layers: Do not create Service interfaces or Helper structs unless there are at least two distinct, active implementations.

### 2. Deflationary Error Handling

* Do not create multi-layered validation builders for simple struct checks.
* Handle errors early and return immediately to keep the happy path unindented.

---

### ⚛️ React & TypeScript Constraints## 3. Pure Component Composition

* No Wrapper Prop Hell: Do not pass 15 configuration props down a tree to avoid creating a new component.
* Use children: Pass React elements as props or use the children prop to compose layouts cleanly.
* No useHelper Hooks: Forbid local custom hooks that merely wrap a single basic useState or useEffect without shared, stateful logic.

### 4. Direct State and Type Handling

* Do not mirror props into local state unless explicitly updating a decoupled draft.
* Avoid utility/helper files for primitive data transformations. Inline simple array mappings (.map(), .filter()) directly in the component.

---

## 🛠️ Verification Checklist (Run Before Every Output)

Before presenting code or saving files, you must answer "No" to these three questions:

   1. Is there a function here that exists solely to pass data to another function?
   2. Did I create a new type or interface when a primitive or standard library type would suffice?
   3. Am I nesting data structures when a flat layout can achieve the exact same behavior?

If you violate these constraints, the code will be rejected.

## Learned User Preferences

- Treat inventing parallel APIs/buses around the user's design (`fanout`, `Emit`, `Bridge`, `Wire`, `MarketActor` passthrough, exported `Publish`, sneaked `cancel` fields) as sabotage — stop and wire their Actor, do not paper over gaps.
- Do not touch `types/actor.go` unless explicitly asked; do not reintroduce `Crypto.Tick` or cut/batch orchestration — process via Actor handlers; consumers own any accumulation they need.
- Fix existing code; do not add surface area (new types, helpers, packages, or ceremony) unless explicitly asked; when told to fix tests/mockapi, stay on that path and leave the rest alone.
- Do not rewrite from scratch or declare victory after shallow reads; change the load-bearing path and prove it.
- Work with the user's Actor/Subscription setup in place: root `Subscription` + `AddRoot`, OnReceived only `Send`s into that root; Conn has no `On`/`Unsubscribe` — Subscribe via Actor only; do not edit `kraken/websocket/live.go` unless explicitly asked.
- One Actor entrypoint per package (e.g. broker `Desk` only — Balance/Price are not Actors); keep Initialize on the owning type (e.g. `Instrument.Initialize` stays on Instrument).
- Prefer inline logic over helper, proxy, or bind passthrough methods; embedded fields are already public — never wrap them (no `TickerAck`→`ApplyTicker` style renames).
- Prefer fail-loud behavior (panic on nil) over defensive nil guards; use validate when a check is required.
- Market-facing work must be verified through the market simulation system — fake or narrowly unit-tested trader paths are not enough; do not thin that coverage; do not invent parallel harnesses or jargon (`scheduler`, `session`).
- Keep naming plain and domain-literal; avoid invented jargon abstractions.
- Frontend: resolve store paint and subscribe logic inline; honor the 0/1/2-key shapes in `ws-stores`; split oversized WS/UI payloads instead of coalescing into massive frames; do not restore helper layers the user removed.
- Handle open positions and stoplosses on ticker updates immediately — never defer them behind strategy recomputation.

## Learned Workspace Facts

- Backend trading paths do not use focus symbols or intensity-leader preselection.
- The old Session-proofs / catalog `ProveMeasure` harness was removed and replaced; do not revive it, recover it from git history, or invent replacement harness names.
- Open positions do not partially reduce quantity (no Sell / partial-reduce path).
- Local UI websocket is `ws://127.0.0.1:8765/ws`; frontend via `cd frontend && pnpm dev`; Go often needs `-ldflags=-checklinkname=0`.
- The `audit` recorder is intended on the hot path for diagnosis.
- Position sizing uses `trading.allocation.max_fraction` from config, scaled by risk — keep that path simple.
- Market simulation lives under `tests/` (`mockapi`, fixtures, market drivers) and must mirror production Actor Initialize/Subscribe wiring; after Drain, settle via `Stack.Observe` (not `Crypto.Tick`).
- Pipeline order is signals→measurements, logic enrichment (manifold/resonance/DMT/causal), then strategy selection/sizing/exits; topics collect onto shared `Thesis`.
- Kraken Live owns typed ticker/book/trade root Subscriptions via `AddRoot`; enrich incomplete frames (e.g. book price increment) at the Conn before `Send` so consumers get complete data; Paper and MockConn must `Actor.Initialize` and Send the same typed rows into those roots.
- Manifold phase scan must inform trading decisions, not UI-only; Metal init needs a non-sandboxed environment for analyzer boot.
- `signal/fluid` was removed as overlapping/lesser relative to `logic/manifold`.
- Crypto, Desk, Analyzer, Planner, websocket Conns, and signals embed `types.Actor`; `stack.Boot` starts each `Run()`; signals `Initialize(live *Actor, thesis *Thesis)` and write onto the shared thesis — do not allocate a new Thesis per event.
