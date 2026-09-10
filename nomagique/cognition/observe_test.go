package cognition_test

import (
	"testing"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/cognition"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/transport"
)

func learn(
	t *testing.T,
	radix *store.Radix,
	assoc cognition.Association,
) *iradix.Tree[[]byte] {
	t.Helper()
	observe := cognition.NewObserve()
	write, err := observe.Record(cognition.ObserveInput{Tree: radix.Read(), Association: assoc})
	if err != nil {
		t.Fatal(err)
	}

	tree, err := transport.Evaluate(radix, transport.Values(observe.Map(write)))
	if err != nil {
		t.Fatal(err)
	}

	return tree
}

func TestObserveLearnsAPrecursor(t *testing.T) {
	config := cognition.DefaultConfig()
	radix := store.NewRadix(iradix.New[[]byte]())

	observe := func(step uint64, context, class string, grade ...float64) {
		t.Helper()
		assoc := cognition.Association{
			Context:   []byte(context),
			Class:     []byte(class),
			Step:      step,
			Retention: config.DecayFactor(),
		}

		if len(grade) > 0 {
			assoc.Feedback = grade[0]
			assoc.Graded = true
		}

		learn(t, radix, assoc)
	}

	for step := uint64(1); step <= 5; step++ {
		observe(step, "A\x00B\x00C", "enter")
	}
	observe(6, "A\x00B\x00C", "wait")

	tree := radix.Read()
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

func TestEvaluateRecallsAPrecursor(t *testing.T) {
	config := cognition.DefaultConfig()
	radix := store.NewRadix(iradix.New[[]byte]())
	recall := cognition.NewEvaluate()

	observe := func(step uint64, context, class string) {
		t.Helper()
		learn(t, radix, cognition.Association{
			Context:   []byte(context),
			Class:     []byte(class),
			Step:      step,
			Retention: config.DecayFactor(),
		})
	}

	evaluate := func(context string) cognition.Evaluation {
		t.Helper()
		reading, err := recall.Recall(cognition.EvaluateInput{
			Tree: radix.Read(),
			Evaluation: cognition.Evaluation{
				Context: []byte(context), Config: config, Step: 32,
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		return reading
	}

	for step := uint64(1); step <= 8; step++ {
		observe(step, "A\x00B\x00C", "enter")
	}
	observe(9, "A\x00B\x00C", "wait")

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

	if unseen := evaluate("Z\x00A\x00B\x00C"); unseen.WinnerClass != "enter" {
		t.Fatalf("expected the suffix to answer, received %q", unseen.WinnerClass)
	}

	if foreign := evaluate("X\x00Y"); foreign.WinnerClass != "" {
		t.Fatalf("expected no association, received %q", foreign.WinnerClass)
	}
}
