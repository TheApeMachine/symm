# cognition

An immutable radix trie for associative learning and inference. Basin and sensory
records use the same pointer-free, 24-byte `PackedWeight` representation.

```go
var root atomic.Pointer[iradix.Tree[[]byte]]
root.Store(iradix.New[[]byte]())
var stepCounter atomic.Uint64

reinforce := cognition.NewReinforce(&root, &stepCounter)
evaluator := cognition.NewEvaluator(&root, &stepCounter)

// Learn an observed association.
assoc := cognition.Association{
	Context: precursorSequence,
	Class:   []byte("action_enter"),
}
reinforce.Next(...)

// Or apply a completed replay grade to that association.
gradedAssoc := cognition.Association{
	Context:  precursorSequence,
	Class:    []byte("action_enter"),
	Feedback: grade * authority,
	Graded:   true,
}
reinforce.Next(...)

// Live inference reads the immutable trie without acquiring a mutex.
evaluator.Next(...)
```

Positive feedback strengthens the action's basin; negative feedback inhibits it.
Zero feedback records sensory context without reinforcing an action. Weights are
empirical association masses and integer observation counts, not calibrated profit
probabilities. There is no separate outcome namespace or return estimator in this package.

`Training.Next` consumes completed grid regions. It retains the precursor sequence,
uses empirical Welford surprisal dispersion to detect sequence breaks (`IsBreak`),
matches `WinnerClass` against legal actions, and reports confidence and contrast.
Missing observations leave the learner waiting; unseen contexts produce no action.

Inference traverses exact basin prefixes and empirical continuation branches.
Immutable reads avoid mutexes; sufficient statistics update atomically via CAS.
