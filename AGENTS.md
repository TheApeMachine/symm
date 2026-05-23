# AGENTS.md

This document defines how coding agents work on this platform. It is a contract, not a style guide. Sections are ordered by priority: the Backend Implementation Contract and Definition of Done come first because they are the rules most often violated.

---

## 1. Backend Implementation Contract

This platform is a crypto trading system, using the Kraken API.

---

## 2. Definition of Done

Work is not complete until verified. Verification means:

- The tests that would catch the bug you are claiming to have fixed have been written and pass.
- For signals, they are correct (no magic numbers, or guessed values, only dynamically derived to respond to market conditions), and actually being used.
- A benchmark exists and has been run.
- The actual test and benchmark output is pasted in the message claiming completion.

Do not say "done" without the proof. Do not say "implemented" without the proof. If a path is incomplete, say so plainly and describe what is missing.

---

## 3. Interaction

1. Do not explain the system back to the user. They built it. If you need to confirm understanding, do it by naming specific files and types, not by summarizing the architecture.

2. Execute the literal request. Not a generalized version, not a "while we're here" expansion, not a smaller version because the full thing seems like a lot. The literal request.

3. Opinions only on request. If the user asks "should I do X", answer. Otherwise do X.

4. Existing structure is load-bearing until proven otherwise. Before replacing or rewriting something, read it and identify what it does. If you cannot explain why the existing code is wrong, do not replace it.

5. Never run `git checkout`, `git reset --hard`, `git restore` against files with uncommitted changes, or any command that discards working tree state. History goes backward; the work goes forward. If you think you need to revert, stop and ask.

6. If you are lost, drifting, or about to do something you are not sure about: stop and say so. Do not paper over uncertainty with confident prose.

7. Do not declare work complete unless you have verified it per Section 2. Paste the output.

---

## 4. Before Writing Code

In order:

1. Read the relevant existing code. Do not propose changes until you can name the files and types involved.
2. Identify what can be removed or refactored to achieve the goal. State this explicitly before adding anything new.
3. Generate at least three solution approaches internally. Discard the first two. Implement the third unless you can explain why an earlier one is strictly better on correctness and performance.
4. If the best solution is large, write it in full. Do not stage it as "minimal version now, real version later." There is no later.

Time-to-deliver, implementation complexity, and scope size are not valid reasons to choose a worse solution. Correctness and performance are the only tiebreakers.

You can write substantial, complete code in one pass when the design is clear. Do so when appropriate. "Fully realized" means correct and verified, not "looks plausible." If the design is not clear, or if you are about to fabricate a part you do not actually know how to write, stop and surface that instead of generating something that resembles the answer.

---

## 5. Code Style

### Structure

Prefer methods over functions. A good codebase is logically spread out into types that define methods, and which are composed together. Objects should look like this:

```go
package packagename

/*
ObjectName is something descriptive.
It also has a reason why it was implemented.
*/
type ObjectName struct {
    ctx    context.Context
    cancel context.CancelFunc
    err    error
}

/*
NewObjectName instantiates a new ObjectName.
It also has a reason for being instantiated.
*/
func NewObjectName(ctx context.Context) *ObjectName {
    ctx, cancel := ctx.WithCancel(ctx)

    return &ObjectName{
        ctx:    ctx,
        cancel: cancel,
    }
}

/*
MethodName.
*/
func (objectName *ObjectName) MethodName() {
    return
}
```

When it comes to TypeScript:

```tsx
/*
Incorrect
*/
export function PaperEditorApp() {
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

/*
Correct
*/
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

Use `const` over `function` and always use the `flex`, `grid`, and `typography` components, over the standard HTML versions.

### Size limits

- **File size:** target 200 lines, hard ceiling 400. At 400+, split before adding more. This does not apply to documentation or custom compute kernels.
- **Method size:** target under 30 lines. Methods over 60 lines must be decomposed unless the operation is genuinely atomic (e.g. a single assembly kernel body).
- **Type size:** if a type has more than ~10 methods, it is doing more than one thing.

### Control flow

- Guard clauses with early return. The happy path stays at indent level 1.
- `else` is not used. If you reach for `else`, invert the condition and return early, or restructure.
- Nested `if` beyond two levels is not allowed. Extract a method or restructure the data so the branch disappears.
- No silent fallbacks. If a precondition fails, return an error. Do not substitute a default and continue.
- Treat `if` as something to minimize. Many branches disappear once you reverse the condition or restructure the data.

### Naming and formatting

- Never use single-character variable names. Receivers included.
- Separate logical code blocks with an empty newline.
- Long function signatures break across lines so that no line crosses the vertical split-view boundary.
- Use modern Go: `maps.Copy`, `for range N`, `for b.Loop()`, etc.

### Density

Prefer compact code that a reader fluent in Go and the relevant ISA can follow. Density is fine. Obscurity for its own sake is not. Less code is better than more code, but only when correctness and performance hold.

If less code means less performance, choose performance.

### Fallbacks or Silent Failures/Errors

Never ever use a fallback or silent errors/faliures. If things are not as they are supposed to be, then return an error properly, and let the code fail. That is the only way we become aware of them so we can fix things.

---

## 6. Testing

Every code file has a `_test.go` mirror. Test function names mirror method names with a `Test` prefix. If you want to test something that does not correspond to a method, the test belongs at the calling site, not in a new free-floating test function.

**Structure:** GoConvey-based, "Given X" / "It should Y", nested.

**Coverage requirements:**

- Mocks are a last resort. Prefer real subsystems wired up in test setup. If you find yourself writing a mock, ask whether the real thing is available; it usually is.

A test that does not meaningfully exercise the code is worse than no test because it provides false confidence. If you cannot articulate what a test proves, delete it.

Keep the README.md up to date alongside test and code changes.

---

## 8. Common Failure Modes

Concrete before/after examples of patterns that have caused regressions on this platform. Read these as the literal list of things not to do.

### Dismissing failing tests as unrelated

```
// Incorrect:
"The X tests are failing but appear unrelated to my changes."

// Correct — all failing tests are in scope. Investigate before continuing.
// It does not matter why a test is failing, what matters is that we don't
// ignore it.
```

### Block separation

```go
// Incorrect
sensoriumOutputs, ok := results.Value.([]*tensors.Tensor)
if !ok || len(sensoriumOutputs) == 0 {
    return "", validate.Require(map[string]any{
        "sensorium_outputs": sensoriumOutputs,
    })
}

// Correct — separate logical blocks with an empty newline
sensoriumOutputs, ok := results.Value.([]*tensors.Tensor)

if !ok || len(sensoriumOutputs) == 0 {
    return "", validate.Require(map[string]any{
        "sensorium_outputs": sensoriumOutputs,
    })
}
```

### Single-character receivers

```go
// Incorrect
func (o *ObjectName) MethodName() { return }

// Correct
func (objectName *ObjectName) MethodName() { return }
```

### Manual loops where the stdlib has it

```go
// Incorrect
for identifier, binding := range rawMap {
    parser.vars[identifier] = binding
}

// Correct
maps.Copy(parser.vars, rawMap)
```

### Long signatures running off-screen

```go
// Incorrect
func (operationRegistry *OperationRegistry) Build(operationID string, config map[string]any) (operation.Operation, error) {

// Correct
func (operationRegistry *OperationRegistry) Build(
    operationID string, config map[string]any,
) (operation.Operation, error) {
```

### Outdated Go idioms

```go
// Incorrect
for range b.N {
    _ = NewErrnieConfig()
}

// Correct
for b.Loop() {
    _ = NewErrnieConfig()
}
```

---

## 9. Reading Order

When starting a task on this codebase, read in this order:

1. This document.
2. `README.md` in the repo root.
3. The package(s) directly relevant to the task.
4. The test files for those packages, to understand the existing contract.

Then reason through the task before writing code. If something in the existing code looks wrong, read it carefully before concluding it is wrong — the user is building toward a goal and existing structure is usually load-bearing.

## 10. Ambiguity Resolution

Always keep the following non-negotiable rules in mind.

1. Accuracy and Performance are the primary concerns, always. If we compromise on Accuracy or Performance, there is no point for anyone to use this framework.
2. You should NOT optimize for the path of least resistance, just to get tests green, or compiler errors resolved. Optimize for Accuracy, Performance, and Maintainability.
3. If you notice you are drifting to any kind of escape hatch, or less than optimal solution, stop, reconsider, and make better choices.
