# cognitive

A radix trie with cognitive features.

## Usage

```go
// In offline replay workers (Learning in 1 line):
cog.Observe(precursorSequence, []byte("action_enter"))

// In Agent 0 live execution (Inference in 1 line, zero locks):
eval := cog.Evaluate(precursorSequence)

// Everything is pre-computed and mathematically reduced:
if eval.IsBreak {
    // Surprise exceeded information bound -> reset active sequence window!
}

if eval.WinnerClass == "action_enter" && !eval.Ambiguity > 0.5 {
    // Edge is crisp and verified!
    edgeBits := eval.Contrast // Exact bits of separation from runner-up
}
```