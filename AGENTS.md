# AGENTS.md

This document defines how coding agents operate on this platform. It is a strict contract, not a style guide. Sections are ordered by execution priority.

---

## 1. Sequence of Operations (Before Writing Code)

When a task is assigned, follow these steps in order before modifying any files:

1. **Code Inventory:** Identify and name the exact files and types involved in the change.
2. **Refactoring Identification:** Explicitly state what can be removed or simplified before writing new code.
3. **Approach Evaluation:** Formulate three distinct solution approaches. Select the approach that maximizes execution performance and structural correctness.
4. **Scope Control:** Execute the literal request. Do not implement generalized abstractions, auxiliary helper files, or out-of-scope modifications.

---

## 2. Backend Implementation Contract

This platform is a cryptocurrency trading system integrated with the Kraken API.

### Signal Integrity and Dynamic Calculations

Hardcoded thresholds, static multipliers, or guessed parameters are not permitted. All logic must dynamically adjust to real-time market data.

#### Incorrect (Magic Numbers)

```go
// This uses an arbitrary, hardcoded percentage threshold
func (signalCalculator *SignalCalculator) IsSignalTriggered() bool {
    threshold := 0.015 
    return (signalCalculator.CurrentPrice - signalCalculator.EntryPrice) / signalCalculator.EntryPrice > threshold
}
```

#### Correct (Dynamically Derived)

```go
// This derives the threshold dynamically using Average True Range (ATR) to adjust to market volatility
func (signalCalculator *SignalCalculator) IsSignalTriggered(averageTrueRange float64) bool {
    if signalCalculator.EntryPrice == 0 {
        return false
    }

    volatilityMultiplier := averageTrueRange / signalCalculator.EntryPrice
    percentageChange := (signalCalculator.CurrentPrice - signalCalculator.EntryPrice) / signalCalculator.EntryPrice

    return percentageChange > volatilityMultiplier
}
```

---

## 3. Definition of Done & Verification

Work is complete only when verified. You must provide proof of execution in your completion message.

* **Automated Tests:** Corresponding test coverage must exist, run, and pass for the exact code path changed.
* **Benchmarks:** A performance benchmark must exist and be executed for any data-processing or signal-calculation changes.
* **Verification Output:** You must paste the literal, unmodified stdout output of the test and benchmark runs in your response.

### Preventative Rules:

* **No Fabrication:** If tool or environment limitations prevent you from executing tests or benchmarks, state: `VERIFICATION LIMITATION: UNABLE TO RUN TESTS` and list the exact terminal commands you would run. Do not write mock or simulated test results.
* **Failing Tests:** If tests fail, you must stop and fix the code. Do not proceed or mark a task complete if any suite is failing.

---

## 4. Code Style & Architecture

### Structure

Prefer methods over functions. Compose types to represent logical units.

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

### Size Limits

* **File Size:** Target 200 lines; hard ceiling of 400 lines. Split files exceeding 400 lines into separate types/files.
* **Method Size:** Target under 30 lines. Methods exceeding 60 lines must be split into sub-methods, unless the operation is atomic (e.g., assembly kernels).
* **Type Size:** Limit types to a maximum of 10 methods.

### Control Flow

* **Early Returns:** Write guard clauses with early returns. Keep the primary logic path at indentation level 1.
* **No Else Blocks:** Do not use `else`. Invert conditions to return early or exit.
* **Nesting Ceiling:** Do not nest `if` blocks deeper than two levels. Extract deeply nested logic into a helper method.
* **No Silent Failures:** If a precondition fails or an unexpected state occurs, return a descriptive error. Substituting default fallbacks or silently skipping errors is prohibited.

### Naming & Formatting

* **No Single-Character Names:** Variable names and method receivers must be descriptive (e.g., use `signalCalculator`, not `s`).
* **Block Separation:** Insert an empty newline between distinct logical code blocks.
* **Line Breaks:** Wrap long function signatures to prevent lines from running past split-view boundaries.

---

## 5. Environment & Tooling Constraints

### Git State Integrity

* Do not read, query, or reference git history, commit logs, or previous branches to solve bugs. Base your solution entirely on the current state of the codebase.
* Never run `git checkout`, `git reset`, `git restore`, or any command that discards working tree changes. If a revert is required, stop and request user intervention.

### Compiler Configuration & Linker Errors

* **dropg Linker Error:** If you encounter a `dropg` linker error, refer to the `Makefile` located in the project root to ensure environment flags and compiler options match the project targets. Do not bypass build constraints with temporary flags.

---

## 6. Interaction Protocol

1. **No Summarization:** Do not explain the existing system architecture back to the user. Reference specific file names and types when discussing changes.
2. **Opinions on Request Only:** Provide design opinions or alternative paradigms only when explicitly asked. Otherwise, implement the requested change directly according to this contract.
3. **Preserve Load-Bearing Structure:** Read and trace existing code paths before proposing modifications. Do not rewrite structural components unless you can document exactly why the existing implementation is broken or incorrect.