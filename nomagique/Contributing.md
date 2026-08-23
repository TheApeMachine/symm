# nomagique Contribution Guide

> no-magique (no magic)
> Number

## nomagique architecture

`nomagique` separates identity, role, and meaning.

- A `Frame` stores named facts and committed state.
- A `Primitive` performs one operation over local structural ports.
- `Wire` explicitly maps named facts to those ports and projects results back.
- No primitive infers operands from alternative names or silently coerces one statistic into another.
- `Pipe` is sequential composition. 
- `Fork` is genuine fan-out: every branch sees the same input and state. 
- `ForkStrict` rejects both state and output collisions.
- A failed transition never commits candidate state.
- `Number` is the keyed top-level owner of composed state.

Domain adapters may name prices, quantities, times, or instruments. Primitive packages should use only mathematical or structural language.

> Domain adapters are generally considered to be bad practice, and it is recommended to translate your domain into generic/abstract terms as soon as possible, ideally as the first step.

## 🚨 The Golden Rule

**No magic numbers. No performative math. No theatre.**

If a value is not a universal mathematical constant, it must be derived from the market data itself via a primitive. Hardcoded multipliers (`*2`), static time windows (e.g., `60s`), and arbitrary thresholds are strictly prohibited.

---

## 1. The Hierarchy of Composition

To prevent top-down "guessing," all logic must be built from the bottom up:

### A. Atomic Primitives (The Foundation)
- **Definition:** The smallest possible unit of computation. A primitive does one thing (e.g., a `ZScore`, an `EMA`, a `Clamp`, or a `Ratio`).
- **Constraint:** Primitives must have zero "judgment." They do not decide if a value is "high" or "low"; they simply provide a measurement.
- **Implementation:** Must be rigorously testable in isolation.

### B. Equations (The Presets)
- **Definition:** A specific, named composition of atomic primitives used to represent a common mathematical identity.
- **Constraint:** **Zero implementation code.** An equation is a "wiring diagram" of primitives. If you see a `for` loop or a custom `if/else` block inside an equation, it is a violation.

### C. Algorithms (The Pipelines)
- **Definition:** High-level compositions of equations and primitives used to derive a specific market measurement.
- **Constraint:** Like equations, algorithms must be compositions. They are the "orchestrators" of the primitives, not the source of the math.

---

## 2. Conditioners vs. Judgments

A common failure mode is mixing **Measurement** with **Judgment**.

- **Conditioners (The Signal):** Their only job is to take raw data and condition it into a measurement.
    - *Correct:* "The current energy is 3x the historical baseline."
    - *Incorrect:* "The current energy is high, therefore we are in an Alpha regime."
- **Judgments (The Strategy):** These happen downstream, far away from `nomagique`. They take the measurements provided by the conditioners and make a decision.

**`nomagique` is for Conditioners. It is not for Judgments.**

---

## 3. Anti-Patterns (Zero Tolerance)

The following are strictly forbidden in any `nomagique` implementation:

- **The Agreement Trap:** Accepting a flawed mathematical implementation just because it exists. If the math is "theatre" (i.e., looks impressive but lacks statistical grounding), it must be dismantled.
- **Top-Down Implementation:** Writing a monolithic function in `algo/` that contains the entire logic of a signal.
- **Implicit Assumptions:** Assuming the data follows a specific distribution (e.g., Gaussian) without first measuring that the assumption is true.
- **Magic Denominators:** Using hardcoded constants to normalize a value instead of using a dynamically derived baseline from the data stream.

## 4. Validation
Every single primitive must be validated against the market. If a primitive's output cannot be traced back to a raw market measurement through a transparent chain of atomic operations, it is "magic" and must be removed.
