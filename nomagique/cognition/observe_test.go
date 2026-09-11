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

	entered := found["b/A\x00B\x00C/enter"]
	waited := found["b/A\x00B\x00C/wait"]

	if entered.Count != 5 {
		t.Fatalf("expected five observations of the precursor, received %d", entered.Count)
	}

	if waited.Count != 1 {
		t.Fatalf("expected one observation of the competitor, received %d", waited.Count)
	}

	smoothed := func(weight cognition.PackedWeight) float64 {
		alpha := config.DirichletAlpha
		return (float64(weight.Count)*weight.Probability + alpha) /
			(float64(weight.Count) + alpha*7)
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

func TestEvaluateUncertaintyOnSparseEvidence(t *testing.T) {
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

	// Single observation: N = 1
	observe(1, "CTX1", "enter_long")

	reading, err := recall.Recall(cognition.EvaluateInput{
		Tree: radix.Read(),
		Evaluation: cognition.Evaluation{
			Context: []byte("CTX1"), Config: config, Step: 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if reading.WinnerClass != "enter_long" {
		t.Fatalf("expected winner to be enter_long, got %q", reading.WinnerClass)
	}

	if reading.Confidence >= 0.90 {
		t.Fatalf("confidence on N=1 must express uncertainty, but got %v (near 100%%)", reading.Confidence)
	}

	if reading.Confidence <= 0 {
		t.Fatalf("expected positive confidence on N=1, got %v", reading.Confidence)
	}

	if reading.Support != 1 {
		t.Fatalf("expected support to be 1, got %d", reading.Support)
	}

	if reading.RunnerUp != "prior" {
		t.Fatalf("expected runner-up on single class match to be prior, got %q", reading.RunnerUp)
	}

	// Many observations: N = 20
	for step := uint64(2); step <= 20; step++ {
		observe(step, "CTX2", "enter_long")
	}

	matureReading, err := recall.Recall(cognition.EvaluateInput{
		Tree: radix.Read(),
		Evaluation: cognition.Evaluation{
			Context: []byte("CTX2"), Config: config, Step: 20,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if matureReading.Confidence <= reading.Confidence {
		t.Fatalf("expected mature confidence (%v) > single observation confidence (%v)", matureReading.Confidence, reading.Confidence)
	}

	if matureReading.Confidence < 0.70 {
		t.Fatalf("expected mature confidence to be substantial on N=20, got %v", matureReading.Confidence)
	}
}
