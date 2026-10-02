package broker_test

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
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

	price := broker.NewPrice(context.Background(), nil, nil, nil, normalizer)

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

	price := broker.NewPrice(context.Background(), nil, nil, nil, spot.NewNormalizer())

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

func TestPrice_AllocateEntryAndLiquidate(t *testing.T) {
	Convey("Given a broker.Price with isolated Book and fee", t, func() {
		ctx := context.Background()
		normalizer := spot.NewNormalizer()
		book := broker.NewBook(ctx, normalizer)
		price := broker.NewPrice(ctx, book, nil, nil, normalizer)
		symbol := "BTC/USD"
		feeRate := 0.001 // 0.1%
		price.SetFee(symbol, kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(feeRate * 100)})

		// Populate book with asks:
		// level 1: 50,000 x 0.01 (cost 500)
		// level 2: 50,100 x 0.05 (cost 2,505)
		// bids:
		// level 1: 49,990 x 0.02
		// level 2: 49,900 x 0.05
		payload := &kraken.Level3{
			Channel: "level3",
			Type:    "snapshot",
			Data: []kraken.Level3Data{
				{
					Symbol: symbol,
					Asks: []kraken.Level3Order{
						{
							OrderID:    "a1",
							LimitPrice: decimal.NewFromFloat64(50000.0),
							OrderQty:   decimal.NewFromFloat64(0.01),
							Timestamp:  time.Now().UTC(),
							Event:      "add",
						},
						{
							OrderID:    "a2",
							LimitPrice: decimal.NewFromFloat64(50100.0),
							OrderQty:   decimal.NewFromFloat64(0.05),
							Timestamp:  time.Now().UTC(),
							Event:      "add",
						},
					},
					Bids: []kraken.Level3Order{
						{
							OrderID:    "b1",
							LimitPrice: decimal.NewFromFloat64(49990.0),
							OrderQty:   decimal.NewFromFloat64(0.02),
							Timestamp:  time.Now().UTC(),
							Event:      "add",
						},
						{
							OrderID:    "b2",
							LimitPrice: decimal.NewFromFloat64(49900.0),
							OrderQty:   decimal.NewFromFloat64(0.05),
							Timestamp:  time.Now().UTC(),
							Event:      "add",
						},
					},
					Timestamp: time.Now().UTC(),
				},
			},
		}
		err := book.Update(payload)
		So(err, ShouldBeNil)

		Convey("AllocateEntry enforces 20% virtual cash budget via depth walk", func() {
			referenceCash := decimal.NewFromFloat64(5000.0) // 20% budget = 1,000
			budget := referenceCash.SetScale(decimal.DefaultScale).Div(decimal.NewFromInt64(5))

			cost, allocErr := price.AllocateEntry(symbol, referenceCash)
			So(allocErr, ShouldBeNil)
			So(cost, ShouldNotBeNil)
			So(cost.Quantity, ShouldNotBeNil)
			So(cost.Quantity.Sign(), ShouldBeGreaterThan, 0)
			So(cost.Total, ShouldNotBeNil)

			// Total entry cost (gross + taker entry fee) must NOT exceed the 20% budget (1,000).
			So(cost.Total.Cmp(budget), ShouldBeLessThanOrEqualTo, 0)

			// The entry must walk past level 1 (500) into level 2: quantity > 0.01
			So(cost.Quantity.Cmp(decimal.NewFromFloat64(0.01)), ShouldBeGreaterThan, 0)

			// VWAP entry price must reflect walking into higher ask level (> 50,000)
			So(cost.EntryPrice.Cmp(decimal.NewFromFloat64(50000.0)), ShouldBeGreaterThan, 0)

			Convey("Liquidate walks bid depth carrying exact quantity with fees/slippage", func() {
				netProceeds, grossProceeds, liqErr := price.Liquidate(symbol, cost.Quantity)
				So(liqErr, ShouldBeNil)
				So(grossProceeds, ShouldNotBeNil)
				So(netProceeds, ShouldNotBeNil)

				// Net proceeds must be gross minus taker exit fee
				So(netProceeds.Cmp(grossProceeds), ShouldBeLessThan, 0)
				So(netProceeds.Sign(), ShouldBeGreaterThan, 0)
			})
		})

		Convey("AllocateEntry errors on non-positive reference cash", func() {
			_, allocErr := price.AllocateEntry(symbol, decimal.NewFromFloat64(0))
			So(allocErr, ShouldNotBeNil)

			_, allocErrNeg := price.AllocateEntry(symbol, decimal.NewFromFloat64(-100))
			So(allocErrNeg, ShouldNotBeNil)
		})
	})

	Convey("Given a broker.Price with quote-only pricing", t, func() {
		ctx := context.Background()
		price := broker.NewPrice(ctx, nil, nil, nil, nil)
		symbol := "ETH/USD"
		feeRate := 0.002 // 0.2%
		price.SetFee(symbol, kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(feeRate * 100)})

		// Update ticker quote
		price.Update(&kraken.TickerData{
			Symbol: symbol,
			Ask:    decimal.NewFromFloat64(2500.0),
			Bid:    decimal.NewFromFloat64(2495.0),
		})

		referenceCash := decimal.NewFromFloat64(1000.0) // 20% budget = 200
		budget := referenceCash.SetScale(decimal.DefaultScale).Div(decimal.NewFromInt64(5))

		cost, err := price.AllocateEntry(symbol, referenceCash)
		So(err, ShouldBeNil)
		So(cost, ShouldNotBeNil)
		So(cost.Total.Cmp(budget), ShouldBeLessThanOrEqualTo, 0)

		netProceeds, grossProceeds, liqErr := price.Liquidate(symbol, cost.Quantity)
		So(liqErr, ShouldBeNil)
		So(grossProceeds, ShouldNotBeNil)
		So(netProceeds, ShouldNotBeNil)
		So(netProceeds.Cmp(grossProceeds), ShouldBeLessThan, 0)
	})
}
