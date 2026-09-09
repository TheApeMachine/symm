# cognition

An immutable radix trie for associative learning and inference. Basin and sensory
records use the same pointer-free, 24-byte `PackedWeight` representation.

```go
// Learn an observed association.
cog.Observe(precursorSequence, []byte("action_enter"))

// Or apply a completed replay grade to that association.
// The strategy supplies a signed return relative to initial funding, adjusted
// for unproductive time by Evaluation.Grade and weighted by measured authority.
cog.Observe(precursorSequence, []byte("action_enter"), grade*authority)

// Live inference reads the immutable trie without acquiring a mutex.
evaluation := cog.Evaluate(precursorSequence)
```

Positive feedback strengthens the action's basin; negative feedback inhibits it.
Zero feedback records sensory context without reinforcing an action. Weights are
association strengths, not expected returns or calibrated profit probabilities.
There is no separate outcome namespace or return estimator in this engine.

`Agent.Step` consumes completed grid regions. It retains the precursor sequence,
uses `IsBreak` to reset that active sequence, matches `WinnerClass` against the
environment's feasible actions, and issues the selected action. Offline agents
explore with probability equal to the reported ambiguity; they also explore when
no feasible winner or no competing class has been observed. Live agents abstain
when no feasible winner exists or the leading classes tie. Abstention creates no
wait action and no training sample.

Only completed replay grades train the policy. Position accounting and forward
outcome reporting stay with their existing owners. Missing market or account
observations leave the agent waiting. The engine's `Encode` and `Decode` persist
its configuration, clock and packed records directly; retired formats are
rejected explicitly rather than silently converted.

Inference currently scans the basin records and allocates its readout/lookahead.
Immutable reads avoid mutexes; they do not imply zero allocations or a proven
latency bound. Package benchmarks measure those costs.
