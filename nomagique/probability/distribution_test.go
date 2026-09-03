package probability

import (
	"math"
	"testing"

	"github.com/theapemachine/symm/nomagique"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
vector is a fixed Collection supplying logits to a Distribution.
*/
type vector []types.Number

func (v vector) Values() []types.Number { return v }

func TestDistributionUniformIsMaximallyAmbiguous(t *testing.T) {
	dist := &Distribution{Logits: vector{1, 1, 1, 1}}

	if got := dist.Step(0); math.Abs(float64(got)-0.25) > 1e-12 {
		t.Fatalf("uniform confidence = %v, want 0.25", got)
	}

	for index := range 4 {
		if math.Abs(float64(dist.Probability(index))-0.25) > 1e-12 {
			t.Fatalf("class %d probability = %v, want 0.25", index, dist.Probability(index))
		}
	}

	if math.Abs(float64(dist.Ambiguity())-1) > 1e-12 {
		t.Fatalf("uniform ambiguity = %v, want 1", dist.Ambiguity())
	}

	if math.Abs(float64(dist.Sharpness())) > 1e-12 {
		t.Fatalf("uniform sharpness = %v, want 0", dist.Sharpness())
	}
}

func TestDistributionWinnerAndSimplex(t *testing.T) {
	dist := &Distribution{Logits: vector{0, 5, 1}}

	confidence := dist.Step(0)

	if !dist.Ready() {
		t.Fatal("finite logits must produce a defined distribution")
	}

	if dist.Winner() != 1 {
		t.Fatalf("winner = %d, want 1", dist.Winner())
	}

	total := 0.0
	for _, probability := range dist.Values() {
		total += float64(probability)
	}

	if math.Abs(total-1) > 1e-12 {
		t.Fatalf("probabilities sum = %v, want 1", total)
	}

	if confidence != dist.Probability(1) {
		t.Fatalf("confidence %v must equal the winning probability %v",
			confidence, dist.Probability(1))
	}

	if dist.Ambiguity() >= 1 {
		t.Fatalf("peaked ambiguity = %v, want < 1", dist.Ambiguity())
	}
}

/*
TestDistributionOverflowShift proves the maximum-shift does real work: logits
large enough that a naive exp would overflow must still normalize.
*/
func TestDistributionOverflowShift(t *testing.T) {
	dist := &Distribution{Logits: vector{1000, 1001}}

	dist.Step(0)

	total := 0.0

	for _, probability := range dist.Values() {
		value := float64(probability)

		if math.IsNaN(value) || math.IsInf(value, 0) {
			t.Fatalf("probability %v is not finite", probability)
		}

		total += value
	}

	if math.Abs(total-1) > 1e-12 {
		t.Fatalf("probabilities sum = %v, want 1", total)
	}

	if dist.Winner() != 1 {
		t.Fatalf("winner = %d, want 1", dist.Winner())
	}
}

/*
TestDistributionDegenerateSlot proves the zero-value rule: an omitted Logits
slot has nothing to normalize.
*/
func TestDistributionDegenerateSlot(t *testing.T) {
	dist := &Distribution{}

	if got := dist.Step(5); got != 0 {
		t.Fatalf("empty Distribution = %v, want 0", got)
	}

	if dist.Ready() {
		t.Fatal("an omitted Logits slot must leave the distribution unready")
	}
}

func TestDistributionRejectsUndefinedInput(t *testing.T) {
	for name, logits := range map[string]vector{
		"empty":    {},
		"nan":      {1, types.Number(math.NaN())},
		"infinite": {1, types.Number(math.Inf(1))},
	} {
		dist := &Distribution{Logits: logits}

		if got := dist.Step(0); got != 0 {
			t.Fatalf("%s logits = %v, want 0", name, got)
		}

		if dist.Ready() {
			t.Fatalf("%s logits must leave the distribution unready", name)
		}

		if dist.Ambiguity() != 0 {
			t.Fatalf("%s logits must leave readings at identity", name)
		}
	}
}

/*
TestDistributionReuseIsAllocationFree upholds the zero-allocation invariant
for a steady-state class count.
*/
func TestDistributionReuseIsAllocationFree(t *testing.T) {
	dist := &Distribution{Logits: vector{0.5, 1.5, -0.25}}

	dist.Step(0)

	allocations := testing.AllocsPerRun(100, func() {
		dist.Step(0)
	})

	if allocations != 0 {
		t.Fatalf("Step allocated %v times per run, want 0", allocations)
	}
}

/*
TestDistributionComposesAsChain proves a Distribution is a real Node: it
drops into a Chain and its output flows on to the next stage.
*/
func TestDistributionComposesAsChain(t *testing.T) {
	dist := &Distribution{Logits: vector{0, 4}}

	pipeline := nomagique.Number(&nomagique.Chain{
		A: dist,
		B: nomagique.Identity{},
	})

	if got := pipeline.Step(0); got != dist.Confidence() {
		t.Fatalf("chained output = %v, want the confidence %v",
			got, dist.Confidence())
	}
}

func TestSoftmaxReduction(t *testing.T) {
	if got := Softmax([]types.Number{1, 1, 1, 1}); math.Abs(float64(got)-0.25) > 1e-12 {
		t.Fatalf("uniform Softmax = %v, want 0.25", got)
	}

	if got := Softmax(nil); got != 0 {
		t.Fatalf("empty Softmax = %v, want 0", got)
	}

	if got := Softmax([]types.Number{1, types.Number(math.NaN())}); got != 0 {
		t.Fatalf("non-finite Softmax = %v, want 0", got)
	}

	if got := Softmax([]types.Number{1000, 1001}); got <= 0.5 || got >= 1 {
		t.Fatalf("overflow-shifted Softmax = %v, want within (0.5, 1)", got)
	}
}

var (
	_ types.Node      = (*Distribution)(nil)
	_ types.Reduction = Softmax
)
