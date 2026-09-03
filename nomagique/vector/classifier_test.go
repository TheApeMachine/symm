package vector

import (
	"testing"

	"github.com/theapemachine/symm/nomagique"
)

func testClassifier(t *testing.T) *Classifier {
	t.Helper()

	classifier, err := NewClassifier(
		NewGroup("rising", "flow/pressure", "flow/momentum"),
		NewGroup("falling", "flow/drawdown"),
	)

	if err != nil {
		t.Fatalf("NewClassifier: %v", err)
	}

	return classifier
}

func TestClassifierRequiresCompetingClasses(t *testing.T) {
	if _, err := NewClassifier(NewGroup("only", "a/b")); err == nil {
		t.Fatal("a single class must not compile into a classifier")
	}

	if _, err := NewClassifier(); err == nil {
		t.Fatal("no classes must not compile into a classifier")
	}

	if _, err := NewClassifier(
		NewGroup("a", "x/y"),
		NewGroup("a", "x/z"),
	); err == nil {
		t.Fatal("a repeated class label must be rejected")
	}

	if _, err := NewClassifier(
		NewGroup("a", "x/y"),
		NewGroup("b"),
	); err == nil {
		t.Fatal("a class with no evidence metrics must be rejected")
	}
}

/*
TestClassifierWaitsForCompleteEvidence proves a partial observation is never
classified and never advances the causal standardizers, so a half-filled
frame cannot corrupt the history the next complete one is measured against.
*/
func TestClassifierWaitsForCompleteEvidence(t *testing.T) {
	classifier := testClassifier(t)

	partial := map[string]float64{"flow/pressure": 1.0}

	if classifier.Complete(partial) {
		t.Fatal("a partial observation must not be complete")
	}

	if classifier.Observe(partial) {
		t.Fatal("a partial observation must not be classified")
	}

	if classifier.Read().Ready {
		t.Fatal("a rejected observation must leave no ready reading")
	}

	if missing := classifier.Missing(partial); len(missing) != 2 {
		t.Fatalf("missing = %v, want the two undeclared metrics", missing)
	}

	if classifier.Maturity() != 0 {
		t.Fatalf("maturity = %v, want 0 after only rejected observations",
			classifier.Maturity())
	}
}

/*
TestClassifierSeparatesClasses proves the classifier discriminates: after a
baseline is established, evidence departing upward favours the class
declaring those metrics.
*/
func TestClassifierSeparatesClasses(t *testing.T) {
	classifier := testClassifier(t)

	for range 30 {
		classifier.Observe(map[string]float64{
			"flow/pressure": 10,
			"flow/momentum": 10,
			"flow/drawdown": 10,
		})
	}

	if !classifier.Observe(map[string]float64{
		"flow/pressure": 40,
		"flow/momentum": 40,
		"flow/drawdown": 10,
	}) {
		t.Fatal("a complete observation must be classified")
	}

	reading := classifier.Read()

	if !reading.Ready {
		t.Fatal("a classified observation must produce a ready reading")
	}

	if reading.WinnerLabel != "rising" {
		t.Fatalf("winner = %q, want \"rising\"", reading.WinnerLabel)
	}

	total := 0.0
	for _, probability := range reading.Probabilities {
		total += probability
	}

	if total < 1-1e-9 || total > 1+1e-9 {
		t.Fatalf("probabilities sum = %v, want 1", total)
	}

	if reading.Confidence <= 0.5 {
		t.Fatalf("confidence = %v, want a decisive winner", reading.Confidence)
	}

	if reading.Sharpness != 1-reading.Ambiguity {
		t.Fatalf("sharpness %v must complement ambiguity %v",
			reading.Sharpness, reading.Ambiguity)
	}
}

/*
TestClassifierSharedMetricStandardizedOnce proves a metric declared by more
than one class advances its standardizer exactly once per observation.
*/
func TestClassifierSharedMetricStandardizedOnce(t *testing.T) {
	classifier, err := NewClassifier(
		NewGroup("a", "shared/metric", "a/only"),
		NewGroup("b", "shared/metric", "b/only"),
	)

	if err != nil {
		t.Fatalf("NewClassifier: %v", err)
	}

	observation := map[string]float64{
		"shared/metric": 5,
		"a/only":        5,
		"b/only":        5,
	}

	classifier.Observe(observation)
	classifier.Observe(observation)

	shared, found := classifier.Standardized("shared/metric")

	if !found {
		t.Fatal("a declared metric must carry a standardized reading")
	}

	only, _ := classifier.Standardized("a/only")

	// Both metrics saw identical values over identical histories, so a shared
	// metric stepped twice per observation would diverge from a single-owner one.
	if shared != only {
		t.Fatalf("shared metric z %v diverged from single-owner z %v — "+
			"the shared metric is being stepped more than once", shared, only)
	}
}

func TestClassifierMaturityTracksLeastEvidenced(t *testing.T) {
	classifier := testClassifier(t)

	observation := map[string]float64{
		"flow/pressure": 1,
		"flow/momentum": 2,
		"flow/drawdown": 3,
	}

	classifier.Observe(observation)
	first := classifier.Maturity()

	for range 20 {
		classifier.Observe(observation)
	}

	if classifier.Maturity() <= first {
		t.Fatalf("maturity %v must grow with evidence from %v",
			classifier.Maturity(), first)
	}

	if classifier.Maturity() >= 1 {
		t.Fatalf("maturity = %v, want strictly below certainty",
			classifier.Maturity())
	}
}

/*
TestClassifierComposesAsNode proves the classifier is a real Node: it drops
into a Chain and its confidence flows on to the next stage.
*/
func TestClassifierComposesAsNode(t *testing.T) {
	classifier := testClassifier(t)

	classifier.Observe(map[string]float64{
		"flow/pressure": 1,
		"flow/momentum": 1,
		"flow/drawdown": 1,
	})

	pipeline := nomagique.Number(&nomagique.Chain{
		A: classifier,
		B: nomagique.Identity{},
	})

	if got := pipeline.Step(0); got != classifier.Distribution().Confidence() {
		t.Fatalf("chained output = %v, want the confidence %v",
			got, classifier.Distribution().Confidence())
	}
}

func TestReadingProbabilityLookup(t *testing.T) {
	classifier := testClassifier(t)

	classifier.Observe(map[string]float64{
		"flow/pressure": 1,
		"flow/momentum": 1,
		"flow/drawdown": 1,
	})

	reading := classifier.Read()

	if _, found := reading.Probability("rising"); !found {
		t.Fatal("a declared class must carry a probability")
	}

	if _, found := reading.Probability("undeclared"); found {
		t.Fatal("an undeclared class must not carry a probability")
	}
}
