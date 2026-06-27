package trader

import (
	"testing"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
)

/*
balanceFrame builds a balances artifact shaped the way kraken/frame.Publish
stores one in the tree: role "balances", asset rows under the "asset" key.
*/
func balanceFrame(stamp int64, rows []map[string]any) *datura.Artifact {
	artifact := datura.Acquire("kraken:private", datura.APPJSON)
	artifact.WithRole("balances")
	artifact.WithScope("snapshot")
	artifact.SetTimestamp(stamp)
	artifact.Merge("asset", rows)

	return artifact
}

func TestHoldingsReadsLatestBalancesFromTree(t *testing.T) {
	tree := dmt.NewTree("")

	older := balanceFrame(100, []map[string]any{
		{"asset": "XBT", "balance": 0.1},
		{"asset": "USD", "balance": 5000.0},
	})
	newer := balanceFrame(200, []map[string]any{
		{"asset": "XBT", "balance": 0.25},
		{"asset": "USD", "balance": 4200.0},
	})

	for _, frame := range []*datura.Artifact{older, newer} {
		updated, _, _ := tree.InsertArtifact(frame.Prefix(), frame)

		if updated != nil {
			tree = updated
		}
	}

	balances := holdings(tree)

	if balances == nil {
		t.Fatal("holdings returned nil for a tree with a balances frame")
	}

	// The newer frame must win.
	if got := balances.Inventory["XBT"]; got != 0.25 {
		t.Fatalf("expected latest XBT balance 0.25, got %v", got)
	}

	if got := balances.Inventory["USD"]; got != 4200.0 {
		t.Fatalf("expected latest USD balance 4200, got %v", got)
	}

	if !balances.Held("XBT") {
		t.Fatal("expected XBT to read as held")
	}

	if !balances.Held("BTC/USD") {
		t.Fatal("expected BTC/USD to read as held from XBT base balance")
	}

	if !balances.Held("BTC/EUR") {
		t.Fatal("expected BTC/EUR to read as held from XBT base balance")
	}

	if balances.Held("ETH/USD") {
		t.Fatal("expected ETH/USD to remain unheld")
	}
}

func TestHoldingsNilWhenNoBalances(t *testing.T) {
	if balances := holdings(dmt.NewTree("")); balances != nil {
		t.Fatalf("expected nil holdings for empty tree, got %+v", balances)
	}

	if balances := holdings(nil); balances != nil {
		t.Fatalf("expected nil holdings for nil tree, got %+v", balances)
	}
}
