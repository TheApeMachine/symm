package market

import (
	"testing"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/logic"
)

func TestStoryUpdateScopesActionArtifacts(t *testing.T) {
	story := &Story{
		tree: &logic.Tree{
			Branches: []*logic.Branch{{
				Action: &logic.Action{
					Type:     logic.ActionMarket,
					Side:     logic.SideBuy,
					Fraction: 0.2,
				},
			}},
		},
	}
	measurement := datura.Acquire("measurement", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope("BTC/USD")

	actions := story.Update([]*datura.Artifact{measurement}, &logic.Balances{})

	if len(actions) != 1 {
		t.Fatalf("expected one story action, got %d", len(actions))
	}

	symbol, _ := actions[0].Scope()
	side, _ := actions[0].Role()

	if symbol != "BTC/USD" || side != "buy" {
		t.Fatalf("story action missing symbol or side: %s/%s", symbol, side)
	}

	if payloadSymbol := datura.Peek[string](actions[0], "symbol"); payloadSymbol != "BTC/USD" {
		t.Fatalf("story action payload symbol = %q, want BTC/USD", payloadSymbol)
	}
}
