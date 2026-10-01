package broker

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken"
)

// TestAffordableFormatsBTCUSDViaSeededNormalizer guards the paper-entry path:
// FormatSize must resolve BTC/USD from a Use/Update-loaded Normalizer. An empty
// Normalizer yields "name not found" and training cannot size entry volume.
func TestAffordableFormatsBTCUSDViaSeededNormalizer(t *testing.T) {
	t.Parallel()

	normalizer := spot.NewNormalizer()
	normalizer.Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{
			"BTC": {AltName: "XBT"},
			"USD": {AltName: "USD"},
		},
		NewPairs: map[string]spot.AssetPair{
			"BTC/USD": {
				WSName:        "XBT/USD",
				Base:          "BTC",
				Quote:         "USD",
				LotDecimals:   8,
				LotMultiplier: 1,
			},
		},
	})

	price := NewPrice(context.Background(), nil, nil, nil, normalizer)

	fee := decimal.NewFromFloat64(0.26)
	price.SetFee("BTC/USD", kraken.TradeVolumeFee{
		Fee:    fee,
		Minfee: fee,
		Maxfee: fee,
	})

	unit, err := decimal.NewFromString("100000.0")
	if err != nil {
		t.Fatalf("unit: %v", err)
	}
	cash, err := decimal.NewFromString("1000.0")
	if err != nil {
		t.Fatalf("cash: %v", err)
	}

	qty, err := price.Affordable("BTC/USD", cash, unit)
	if err != nil {
		t.Fatalf("Affordable BTC/USD: %v", err)
	}
	if qty == nil || qty.Sign() <= 0 {
		t.Fatalf("expected positive venue-normalized quantity, got %v", qty)
	}
}

func TestAffordableEmptyNormalizerReportsNameNotFound(t *testing.T) {
	t.Parallel()

	price := NewPrice(context.Background(), nil, nil, nil, spot.NewNormalizer())

	fee := decimal.NewFromFloat64(0.26)
	price.SetFee("BTC/USD", kraken.TradeVolumeFee{
		Fee:    fee,
		Minfee: fee,
		Maxfee: fee,
	})

	unit, err := decimal.NewFromString("100000.0")
	if err != nil {
		t.Fatalf("unit: %v", err)
	}
	cash, err := decimal.NewFromString("1000.0")
	if err != nil {
		t.Fatalf("cash: %v", err)
	}

	_, err = price.Affordable("BTC/USD", cash, unit)
	if err == nil {
		t.Fatal("expected normalize failure with empty Normalizer")
	}
}
