package rest

import "testing"

func TestAssetBalanceFindsPrefixedCode(t *testing.T) {
	balance := &Balance{
		Result: map[string]string{
			"XXBT": "0.125",
		},
	}

	amount, ok := balance.AssetBalance("XXBT")

	if !ok {
		t.Fatal("expected balance lookup")
	}

	if amount != 0.125 {
		t.Fatalf("expected 0.125, got %v", amount)
	}
}
