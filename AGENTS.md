# AGENTS.md

## 🚨 CRITICAL SCOPE LOCK & CIRCUIT BREAKER

1. **STRICT SCOPE BOUNDARY:** Touch ONLY the files explicitly requested or strictly required to solve the task. Do NOT perform unprompted cleanups, renames, or refactorings in neighboring files.
2. **THE "STOP WORK" RULE:** Once the requested task is solved and tests pass, **STOP IMMEDIATELY**. Do not look for "extra work", do not sharpen unrelated logic, and do not reorganize project architecture unless explicitly ordered to do so in the prompt.
3. **ZERO UNCHECKED ABSTRACTIONS:** If a task can be solved in 10 lines of flat Go, write 10 lines of flat Go. Do not create new structs, interfaces, or package structures unless the single-file size ceiling (>400 lines) explicitly mandates it.

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

### Composition & Simplicity

* **Zero Proxy/Passthrough Methods:** Never write a wrapper method on a struct that merely calls a method on an embedded field or another object. Expose embedded types or call them directly.
* **Flatter is Better:** If a method exists solely to pass data to another method in a 1-line chain, delete it and inline the call.
* **Methods Over Loose Functions:** Group logic into cohesive domain types. Keep implementations direct and simple.

### Control Flow & Safety

* **Early Returns & Guard Clauses:** Keep primary logic at indentation level 1.
* **No `else` Blocks:** Invert conditions and return/exit early.
* **Nesting Ceiling:** Max 2 levels of `if` nesting. If logic requires deeper branching, simplify the state checks instead of scattering code into helper methods.
* **No Silent Failures:** Return descriptive errors. Substituting default fallbacks or ignoring errors is prohibited. Let unexpected panics surface rather than hiding them.
* **No checks for NaN or Inf** Prefer to let the system crash, because this will give us the clearest signal possible that somewhere there is something wrong. By handling these cases and letting the system continue, we have silent death in the system. If for example we return 0 as the fallback and then use that in a multiplication, we instantly zero out the operations. Just let the system crash, then we can solve the cause and not the effect.

### Formatting & Naming

* **No Single-Character Variable Names:** Variable receivers and variables must be descriptive (e.g., `signalCalculator`, not `s`). Exception: `t *testing.T` and `b *testing.B`.
* **Line Formatting:** `if` statements MUST have an empty newline above them (unless at the very start of a block). Wrap long parameter lists cleanly across line breaks.
* **Errors & Logging:** Error variables must be named `err`. Log errors strictly via `errnie`:
  ```go
  errnie.Error(errnie.Err(
      errnie.Validation,
      "descriptive error message",
      err,
  ))
* **Test Structure Mirrors Code Structure** Only have test files that mirror the code files (<codefile_test>.go) and only have test methods that mirror the methods in the code file that is being tested (func <CodeMethod>Test(t *testing.T)). Benchmarks follow the same function naming rules and are always at the bottom of the test file. Always use Goconvey for tests, and use BDD-style nesting for your test cases. Treat tests code as you would implementation code, and keep things clean and DRY.

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
			// Verify that the transaction is alive by executing a command.
			_, err := tx.Exec("SELECT 1")
			So(err, ShouldBeNil)

			tx.Rollback()
		})

		// Here we invoke the actual test-closure and provide the transaction.
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

## IMPORTANT

READ THE Makefile TO LEARN HOW TO DEAL WITH THE qpool LINKER ISSUE!

PLEASE DO NOT TRY TO SOLVE EVERY ISSUE BY LOOKING AT GIT HISTORY.
THERE IS A REASON IT IS CALLED HISTORY, AND IT IS RARELY THE WAY
FORWARD, EVEN IF TECHNICALLY AN OLDER SOLUTION WOULD WORK AND "SOLVE"
AN ISSUE, IT WAS OBVIOUSLY REJECTED FOR A REASON.

`/**/` style comments only for top-level structures, inline comments use `//`.

```go
/*
Sample represents a fully populated market ticker payload point.
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