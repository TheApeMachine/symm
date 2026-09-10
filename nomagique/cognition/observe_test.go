package cognition_test

import (
	"testing"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

/*
The learner is one composition: read the store as it stands, move the link the
observation names, write the result back, and keep it.
*/
func TestObserveLearnsAPrecursor(t *testing.T) {
	config := cognition.DefaultConfig()
	retained := store.NewRetained(core.From(iradix.New[[]byte]()))
	seen := store.NewRetained(core.From(map[string][]byte{}))

	learn := transport.NewPipe(
		transport.NewApply(retained, nil),
		cognition.NewObserve(transport.NewApply(seen, nil)),
		store.NewRadix[iradix.Tree[any]](retained),
		retained,
	)

	observe := func(step uint64, context, class string, grade ...float64) {
		t.Helper()
		observation := map[string][]byte{
			"context":   []byte(context),
			"class":     []byte(class),
			"step":      cognition.Counted(step),
			"retention": cognition.Measured(config.DecayFactor()),
		}

		if len(grade) > 0 {
			observation["feedback"] = cognition.Measured(grade[0])
		}
		tests.Drain(t, seen, transport.NewIO(core.From(observation)))
		tests.Drain(t, learn, nil)
		tests.Sound(t, learn)
	}

	// A precursor seen repeatedly, and a competing one seen once.
	for step := uint64(1); step <= 5; step++ {
		observe(step, "A\x00B\x00C", "enter")
	}
	observe(6, "A\x00B\x00C", "wait")

	tree := core.To[*iradix.Tree[[]byte]](transport.NewApply(retained, nil).Next(nil))

	if tree == nil {
		t.Fatal("expected the store to hold what was learned")
	}
	found := map[string]cognition.PackedWeight{}
	iterator := tree.Root().Iterator()
	iterator.SeekPrefix([]byte("b/"))

	for key, value, more := iterator.Next(); more; key, value, more = iterator.Next() {
		found[string(key)] = cognition.DecodeWeight(value)
	}

	if len(found) != 2 {
		t.Fatalf("expected one link per class, received %d: %v", len(found), found)
	}
	entered := found["b/enter/A\x00B\x00C"]
	waited := found["b/wait/A\x00B\x00C"]

	if entered.Count != 5 {
		t.Fatalf("expected five observations of the precursor, received %d", entered.Count)
	}

	if waited.Count != 1 {
		t.Fatalf("expected one observation of the competitor, received %d", waited.Count)
	}

	// Raw strength is not the ranking. An ungraded link opens at one, so a
	// single coincidence reads as certain until its evidence is weighed
	// against the prior standing for having learned nothing at all.
	smoothed := func(weight cognition.PackedWeight) float64 {
		alpha := config.DirichletAlpha

		return (float64(weight.Count)*weight.Probability + alpha) /
			(float64(weight.Count) + alpha*16)
	}

	if smoothed(entered) <= smoothed(waited) {
		t.Fatalf(
			"expected the repeated precursor to outweigh the one-off: enter=%v wait=%v",
			smoothed(entered), smoothed(waited),
		)
	}

	if entered.Probability > waited.Probability {
		t.Fatal("expected raw strength alone not to separate them")
	}
}

/*
What was learned is read back through the same store, and a sequence that was
never stored in full is still answered for by the ones that were.
*/
func TestEvaluateRecallsAPrecursor(t *testing.T) {
	config := cognition.DefaultConfig()
	retained := store.NewRetained(core.From(iradix.New[[]byte]()))
	seen := store.NewRetained(core.From(map[string][]byte{}))
	asked := store.NewRetained(core.From(cognition.Evaluation{}))

	learn := transport.NewPipe(
		transport.NewApply(retained, nil),
		cognition.NewObserve(transport.NewApply(seen, nil)),
		store.NewRadix[iradix.Tree[any]](retained),
		retained,
	)
	recall := transport.NewPipe(
		transport.NewApply(retained, nil),
		cognition.NewEvaluate(transport.NewApply(asked, nil)),
	)

	observe := func(step uint64, context, class string) {
		t.Helper()
		tests.Drain(t, seen, transport.NewIO(core.From(map[string][]byte{
			"context":   []byte(context),
			"class":     []byte(class),
			"step":      cognition.Counted(step),
			"retention": cognition.Measured(config.DecayFactor()),
		})))
		tests.Drain(t, learn, nil)
		tests.Sound(t, learn)
	}

	evaluate := func(context string) cognition.Evaluation {
		t.Helper()
		tests.Drain(t, asked, transport.NewIO(core.From(cognition.Evaluation{
			Context: []byte(context), Config: config, Step: 32,
		})))
		answered := tests.Drain(t, recall, nil)
		tests.Sound(t, recall)

		if len(answered) == 0 {
			t.Fatal("expected the store to answer")
		}
		reading, held := answered[len(answered)-1].(cognition.Evaluation)

		if !held {
			t.Fatalf("expected a reading, received %T", answered[len(answered)-1])
		}

		return reading
	}

	for step := uint64(1); step <= 8; step++ {
		observe(step, "A\x00B\x00C", "enter")
	}
	observe(9, "A\x00B\x00C", "wait")

	// The sequence it was taught on.
	reading := evaluate("A\x00B\x00C")

	if reading.WinnerClass != "enter" {
		t.Fatalf("expected the learned precursor, received %q", reading.WinnerClass)
	}

	if reading.RunnerUp != "wait" {
		t.Fatalf("expected the competitor as runner-up, received %q", reading.RunnerUp)
	}

	if reading.Contrast <= 0 {
		t.Fatalf("expected the winner ahead of the runner-up, received %v", reading.Contrast)
	}

	if reading.Support != 8 {
		t.Fatalf("expected the evidence behind the winner, received %d", reading.Support)
	}

	// A longer sequence that was never stored: it ends with one that was, so
	// what was learned there still answers for it.
	if unseen := evaluate("Z\x00A\x00B\x00C"); unseen.WinnerClass != "enter" {
		t.Fatalf("expected the suffix to answer, received %q", unseen.WinnerClass)
	}

	// A sequence sharing nothing reaches no stored link at all.
	if foreign := evaluate("X\x00Y"); foreign.WinnerClass != "" {
		t.Fatalf("expected no association, received %q", foreign.WinnerClass)
	}
}
