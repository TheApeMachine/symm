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

func TestSaleFollowsTheBidWhenTheBookCannotFill(t *testing.T) {
	t.Parallel()

	price := broker.NewPrice(context.Background(), nil, nil, nil, spot.NewNormalizer())
	symbol := "FET/USD"
	fee := decimal.NewFromFloat64(0.26)
	price.SetFee(symbol, kraken.TradeVolumeFee{Fee: fee, Minfee: fee, Maxfee: fee})

	entry := decimal.NewFromFloat64(100)
	qty := decimal.NewFromFloat64(2)
	ask := decimal.NewFromFloat64(101)

	price.SetQuote(symbol, decimal.NewFromFloat64(110), ask)
	sale, _, ret, ok := price.Sale(symbol, qty, entry, nil)

	if !ok {
		t.Fatal("a quoted bid did not mark the lot")
	}

	if sale != 110 || ret <= 0 {
		t.Fatalf("sale = %v ret = %v, want the higher bid and a gain", sale, ret)
	}

	price.SetQuote(symbol, decimal.NewFromFloat64(90), ask)
	sale, _, ret, ok = price.Sale(symbol, qty, entry, nil)

	if !ok || sale != 90 || ret >= 0 {
		t.Fatalf("sale = %v ret = %v, want the lower bid and a loss", sale, ret)
	}
}

/*
The fee legs are tier rates in percent, the units the fee table speaks in: the
entry rate rides the cost and the exit rate rides the proceeds. A flat price
must cost the round trip and nothing more — a rate read as a dollar amount
turns a flat lot into a large loss.
*/
func TestSaleAppliesTierRatesToCostAndProceeds(t *testing.T) {
	t.Parallel()

	price := broker.NewPrice(context.Background(), nil, nil, nil, spot.NewNormalizer())

	symbol := "AAVE/USD"
	tier := decimal.NewFromFloat64(1.587)
	price.SetFee(symbol, kraken.TradeVolumeFee{Fee: tier, Minfee: tier, Maxfee: tier})

	entry := decimal.NewFromFloat64(0.5)
	qty := decimal.NewFromFloat64(10)
	bid := decimal.NewFromFloat64(0.5)
	ask := decimal.NewFromFloat64(0.51)

	price.SetQuote(symbol, bid, ask)

	_, _, ret, ok := price.Sale(symbol, qty, entry, tier)

	if !ok {
		t.Fatal("a quoted bid did not mark the lot")
	}

	// Both sides of a 1.587% tier on a flat price.
	want := -0.03124

	if ret < want-0.002 || ret > want+0.002 {
		t.Fatalf("ret = %v, want the round trip of two tier rates (%v)", ret, want)
	}
}

func TestSaleUsesTheBookWhenItCanTakeTheQuantity(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	normalizer := spot.NewNormalizer()
	tickSize, err := decimal.NewFromString("0.1")

	if err != nil {
		t.Fatal(err)
	}

	normalizer.Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{"BTC": {AltName: "BTC"}, "USD": {AltName: "USD"}},
		NewPairs: map[string]spot.AssetPair{"BTCUSD": {
			WSName: "BTC/USD", Base: "BTC", Quote: "USD",
			PairDecimals: 1, LotDecimals: 8, LotMultiplier: 1, TickSize: tickSize,
		}},
	})

	book := broker.NewBook(ctx, normalizer)
	price := broker.NewPrice(ctx, book, nil, nil, normalizer)
	symbol := "BTC/USD"
	fee := decimal.NewFromFloat64(0.1)
	price.SetFee(symbol, kraken.TradeVolumeFee{Fee: fee, Minfee: fee, Maxfee: fee})
	price.SetQuote(symbol, decimal.NewFromFloat64(1), decimal.NewFromFloat64(2))

	err = book.Update(&kraken.Level3{
		Channel: "level3",
		Type:    "snapshot",
		Data: []kraken.Level3Data{{
			Symbol: symbol,
			Asks: []kraken.Level3Order{{
				OrderID: "a1", LimitPrice: decimal.NewFromFloat64(50000),
				OrderQty: decimal.NewFromFloat64(1), Timestamp: time.Now().UTC(), Event: "add",
			}},
			Bids: []kraken.Level3Order{
				{
					OrderID: "b1", LimitPrice: decimal.NewFromFloat64(49990),
					OrderQty: decimal.NewFromFloat64(0.02), Timestamp: time.Now().UTC(), Event: "add",
				},
				{
					OrderID: "b2", LimitPrice: decimal.NewFromFloat64(49900),
					OrderQty: decimal.NewFromFloat64(0.05), Timestamp: time.Now().UTC(), Event: "add",
				},
			},
			Timestamp: time.Now().UTC(),
		}},
	})

	if err != nil {
		t.Fatal(err)
	}

	sale, _, _, ok := price.Sale(symbol, decimal.NewFromFloat64(0.04), decimal.NewFromFloat64(50000), nil)

	if !ok {
		t.Fatal("a book deep enough for the quantity did not price the sale")
	}

	if sale >= 49990 || sale <= 49900 {
		t.Fatalf("sale = %v, want the walk through both bids", sale)
	}
}

func TestPrice_AllocateEntryAndLiquidate(t *testing.T) {
	Convey("Given a broker.Price with isolated Book and fee", t, func() {
		ctx := context.Background()
		normalizer := spot.NewNormalizer()
		tickSize, err := decimal.NewFromString("0.1")
		So(err, ShouldBeNil)
		normalizer.Update(&spot.AssetsManagerUpdate{
			NewAssets: map[string]spot.AssetInfo{"BTC": {AltName: "BTC"}, "USD": {AltName: "USD"}},
			NewPairs: map[string]spot.AssetPair{"BTCUSD": {
				WSName: "BTC/USD", Base: "BTC", Quote: "USD",
				PairDecimals: 1, LotDecimals: 8, LotMultiplier: 1, TickSize: tickSize,
			}},
		})
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
		err = book.Update(payload)
		So(err, ShouldBeNil)

		Convey("AllocateEntry spends at most the reference cash via depth walk", func() {
			referenceCash := decimal.NewFromFloat64(1000.0)
			budget := referenceCash.SetScale(decimal.DefaultScale)

			cost, allocErr := price.AllocateEntry(symbol, referenceCash)
			So(allocErr, ShouldBeNil)
			So(cost, ShouldNotBeNil)
			So(cost.Quantity, ShouldNotBeNil)
			So(cost.Quantity.Sign(), ShouldBeGreaterThan, 0)
			So(cost.Total, ShouldNotBeNil)

			// Total entry cost (gross + taker entry fee) must NOT exceed the reference cash (1,000).
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

		// Set quote
		price.SetQuote(
			symbol,
			decimal.NewFromFloat64(2495.0),
			decimal.NewFromFloat64(2500.0),
		)

		referenceCash := decimal.NewFromFloat64(200.0)
		budget := referenceCash.SetScale(decimal.DefaultScale)

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
