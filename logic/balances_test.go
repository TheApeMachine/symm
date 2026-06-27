package logic

import "testing"

func TestBalancesHeldNormalizesPairSymbols(t *testing.T) {
	balances := &Balances{
		Inventory: map[string]float64{
			"USD": 1000,
			"XBT": 0.25,
		},
		Asset: []BalanceAsset{
			{Asset: "ZUSD", Balance: 1000},
			{Asset: "XXBT", Balance: 0.25},
		},
	}

	for _, symbol := range []string{"BTC/USD", "BTC/EUR", "XBT/USD", "XXBT/ZUSD"} {
		if !balances.Held(symbol) {
			t.Fatalf("expected %s to be held from XBT/XXBT base balance", symbol)
		}
	}

	if balances.Held("ETH/USD") {
		t.Fatal("ETH/USD should not be held by quote cash or BTC inventory")
	}
}

func TestBalancesHeldUsesExactPairInventory(t *testing.T) {
	balances := &Balances{
		Inventory: map[string]float64{"ETH/USD": 1.5},
	}

	if !balances.Held("ETH/USD") {
		t.Fatal("expected exact pair inventory to count as held")
	}
}
