package logic

import (
	"context"
	"testing"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
)

func TestOrganicTrendEntryDoesNotRequireDeferredManifold(t *testing.T) {
	ctx := context.Background()
	pool := qpool.NewQ[any](ctx, 1, 2, nil)
	tree, err := NewTree(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}

	measurements := []*datura.Artifact{
		testMeasurementArtifact(SourcePumpDump, "BTC/USD", CategoryOrganicTrend, 0.9, 1),
		testMeasurementArtifact(SourceSentiment, "BTC/USD", CategoryRiskOnSurge, 0.8, 1),
		testMeasurementArtifact(SourceExhaustion, "BTC/USD", CategoryThermalExhaustion, 0.2, 1),
		testMeasurementArtifact(SourceLiquidity, "BTC/USD", CategoryRobustLiquidity, 0.2, 1),
	}

	actions, _ := WalkTreeActions("BTC/USD", measurements, &Balances{}, tree.Branches)
	if len(actions) == 0 {
		t.Fatal("organic trend entry was blocked by missing deferred manifold measurement")
	}

	for _, action := range actions {
		if action.Type == ActionMarket && action.Side == SideBuy && action.Symbol == "BTC/USD" {
			return
		}
	}

	t.Fatalf("organic trend produced no BTC/USD buy action: %#v", actions)
}
