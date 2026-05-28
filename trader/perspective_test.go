package trader

import (
	"testing"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

/*
TestPerspectivePredictAlwaysReturns proves that any non-empty bucket produces
a prediction. Selectivity for trade entry happens downstream — the spec calls
for predictions on every batch so feedback can flow back even when no trade is
opened.
*/
func TestPerspectivePredictAlwaysReturns(t *testing.T) {
	measurement := engine.Measurement{
		Source:     "hawkes",
		Regime:     "cluster",
		Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
		Confidence: 0.3,
		Last:       100,
	}

	perspective := NewPerspective([]engine.Measurement{measurement})
	prediction, err := perspective.Predict(engine.PerspectiveMicrostructure)
	if err != nil {
		t.Fatalf("non-empty bucket should produce a prediction: %v", err)
	}

	if !perspective.Ready {
		t.Fatal("expected perspective marked ready")
	}

	if prediction.Confidence <= 0 {
		t.Fatalf("expected positive fused confidence, got %v", prediction.Confidence)
	}

	if prediction.Perspective.Type != engine.PerspectiveMicrostructure {
		t.Fatalf("expected perspective type preserved, got %v", prediction.Perspective.Type)
	}
}

/*
TestPerspectivePredictEmptyIsNotReady guards the only case where Predict
declines: an empty bucket has nothing to forecast against.
*/
func TestPerspectivePredictEmptyIsNotReady(t *testing.T) {
	perspective := NewPerspective(nil)
	if _, err := perspective.Predict(engine.PerspectiveFlow); err == nil {
		t.Fatal("expected empty perspective to stay not ready")
	}
}
