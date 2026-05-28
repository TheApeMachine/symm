package trader

import (
	"testing"

	"github.com/theapemachine/symm/engine"
	"github.com/theapemachine/symm/kraken/asset"
)

func TestPerspectiveAccumulateThenPredict(t *testing.T) {
	weak := engine.Measurement{
		Source:     "hawkes",
		Regime:     "cluster",
		Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
		Confidence: 0.4,
		Last:       100,
	}

	perspective := NewPerspective([]engine.Measurement{weak})
	_, err := perspective.Predict()

	if err == nil {
		t.Fatal("expected weak single-source perspective to stay not ready")
	}

	perspective.AddMeasurement(engine.Measurement{
		Source:     "leadlag",
		Regime:     "cross_asset",
		Pairs:      []asset.Pair{{Wsname: "BTC/EUR"}},
		Confidence: 0.7,
		Last:       100,
	})

	prediction, err := perspective.Predict()

	if err != nil {
		t.Fatalf("expected fused perspective to become ready: %v", err)
	}

	if !perspective.Ready {
		t.Fatal("expected perspective marked ready")
	}

	if prediction.Confidence <= 0 {
		t.Fatalf("expected positive fused confidence, got %v", prediction.Confidence)
	}
}
