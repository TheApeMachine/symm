package broker

import (
	"sync"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/shopspring/decimal"
	shop "github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/venue/kraken"
)

func TestAllocateCapsAtTouchDivisor(t *testing.T) {
	Convey("Allocate never takes more than best-bid / TouchDivisor", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol:     "AAA/USD",
			cash:       1_000_000,
			qtyMinRaw:  "0.01",
			costMinRaw: "1",
			step:       "0.01",
			bidPx:      99,
			bidQty:     300,
			askPx:      100,
			askQty:     10_000,
			feePct:     0.4,
		})

		qty, err := fixture.price.Allocate(fixture.symbol, fixture.wallet)
		So(err, ShouldBeNil)
		So(qty, ShouldNotBeNil)

		// touchCap = 300 / 3 = 100. Cash is not the binding constraint.
		So(qty.String(), ShouldEqual, "100")
	})
}

func TestAllocateRespectsCashBudget(t *testing.T) {
	Convey("Allocate fee-inclusive cost stays within max_fraction of cash", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol:     "BBB/USD",
			cash:       1000,
			qtyMinRaw:  "0.01",
			costMinRaw: "1",
			step:       "0.01",
			bidPx:      99,
			bidQty:     100_000,
			askPx:      100,
			askQty:     100_000,
			feePct:     0.8,
		})

		qty, err := fixture.price.Allocate(fixture.symbol, fixture.wallet)
		So(err, ShouldBeNil)
		So(qty, ShouldNotBeNil)

		fill, err := fixture.price.EntryCost(fixture.symbol, qty)
		So(err, ShouldBeNil)
		So(fill, ShouldNotBeNil)

		// budget = 1000 * 0.20 = 200. Entry fee is quote paid on top of gross.
		gross := fill.EntryPrice.Mul(*qty)
		total := gross.Add(*fill.EntryFee)
		budget := decimal.NewFromFloat(200)

		So(total.Cmp(budget), ShouldBeLessThanOrEqualTo, 0)

		// With ask 100 and 0.8% fee, gross budget = 200/1.008 ≈ 198.41 → qty ≈ 1.98
		// so the lot must be near that, not the touch cap.
		So(qty.InexactFloat64(), ShouldBeBetween, 1.9, 2.0)
	})
}

func TestAllocateFailsBelowVenueMin(t *testing.T) {
	Convey("Allocate refuses a lot under QtyMin when the touch cap cannot clear it", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol:     "CCC/USD",
			cash:       100_000,
			qtyMinRaw:  "50",
			costMinRaw: "1",
			step:       "1",
			bidPx:      99,
			bidQty:     30, // touchCap = 10
			askPx:      100,
			askQty:     10_000,
			feePct:     0.4,
		})

		qty, err := fixture.price.Allocate(fixture.symbol, fixture.wallet)
		So(qty, ShouldBeNil)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "below venue minimum")
		So(err.Error(), ShouldContainSubstring, "touch_cap=10")
	})
}

func TestAllocateFailsWithoutFee(t *testing.T) {
	Convey("Allocate refuses when the taker fee is unknown", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol:     "NOFEE/USD",
			cash:       1_000_000,
			qtyMinRaw:  "0.01",
			costMinRaw: "1",
			step:       "0.01",
			bidPx:      99,
			bidQty:     300,
			askPx:      100,
			askQty:     10_000,
			feePct:     0.4,
		})

		fixture.price.Instrument.takerFees = &sync.Map{}

		qty, err := fixture.price.Allocate(fixture.symbol, fixture.wallet)
		So(qty, ShouldBeNil)
		So(err, ShouldNotBeNil)
		So(err.Error(), ShouldContainSubstring, "taker fee unknown")
	})
}

func TestFeeSymbolMapsTradeVolumeKey(t *testing.T) {
	Convey("TradeVolume fee keys map onto websocket symbols without alias mis-splits", t, func() {
		xrp := "XRP/USD"
		gala := "GALA/USD"

		instrument := &Instrument{
			cache:     &sync.Map{},
			takerFees: &sync.Map{},
			symbols:   []string{xrp, gala},
		}
		instrument.cache.Store(xrp, kraken.InstrumentPair{
			Symbol: xrp, Base: "XRP", Quote: "USD", Status: "online",
		})
		instrument.cache.Store(gala, kraken.InstrumentPair{
			Symbol: gala, Base: "GALA", Quote: "USD", Status: "online",
		})

		normalizer := spot.NewNormalizer()
		normalizer.Update(&spot.AssetsManagerUpdate{
			NewAssets: map[string]spot.AssetInfo{
				"XRP":  {AltName: "XRP"},
				"GALA": {AltName: "GALA"},
				"GAL":  {AltName: "GAL"},
				"USD":  {AltName: "USD"},
				"AUSD": {AltName: "AUSD"},
			},
			OldAssets: map[string]spot.AssetInfo{
				"XXRP": {AltName: "XRP"},
				"ZUSD": {AltName: "USD"},
			},
			NewPairs: map[string]spot.AssetPair{
				"XRP/USD": {
					AltName: "XRPUSD", WSName: "XRP/USD", Base: "XRP", Quote: "USD",
				},
				"GALA/USD": {
					AltName: "GALAUSD", WSName: "GALA/USD", Base: "GALA", Quote: "USD",
				},
			},
			OldPairs: map[string]spot.AssetPair{
				"XXRPZUSD": {
					AltName: "XRPUSD", WSName: "XRP/USD", Base: "XXRP", Quote: "ZUSD",
				},
			},
		})

		price := &Price{Instrument: instrument, normalizer: normalizer}

		So(price.feeSymbol("XXRPZUSD"), ShouldEqual, xrp)
		So(price.feeSymbol("XRPUSD"), ShouldEqual, xrp)
		So(price.feeSymbol(xrp), ShouldEqual, xrp)
		// Desk-first compact match wins even when alias search is ambiguous.
		So(price.feeSymbol("GALAUSD"), ShouldEqual, gala)

		fee := decimal.NewFromFloat(0.26)
		price.Instrument.RememberFee(price.feeSymbol("XXRPZUSD"), &fee)
		So(price.Fee(xrp), ShouldNotBeNil)
		So(price.Fee(xrp).String(), ShouldEqual, "0.26")
	})
}

func TestAllocateVirtualAllowsUnderVenueMin(t *testing.T) {
	Convey("AllocateVirtual returns a thin learning lot that Allocate rejects", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol:     "DDD/USD",
			cash:       100_000,
			qtyMinRaw:  "50",
			costMinRaw: "1",
			step:       "1",
			bidPx:      99,
			bidQty:     30, // touchCap = 10 < QtyMin
			askPx:      100,
			askQty:     10_000,
			feePct:     0.4,
		})

		venue, venueErr := fixture.price.Allocate(fixture.symbol, fixture.wallet)
		So(venue, ShouldBeNil)
		So(venueErr, ShouldNotBeNil)

		virtual, virtualErr := fixture.price.Allocate(fixture.symbol, nil)
		So(virtualErr, ShouldBeNil)
		So(virtual, ShouldNotBeNil)
		So(virtual.String(), ShouldEqual, "10")
		So(virtual.Cmp(*fixture.qtyMin), ShouldEqual, -1)
	})
}

func TestAllocateUsesPassedWallet(t *testing.T) {
	Convey("Allocate sizes against the wallet argument, not always the desk balance", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol:     "FFF/USD",
			cash:       100_000,
			qtyMinRaw:  "0.01",
			costMinRaw: "1",
			step:       "0.01",
			bidPx:      99,
			bidQty:     100_000,
			askPx:      100,
			askQty:     100_000,
			feePct:     0.4,
		})

		cash := shop.NewFromInt(500)
		small := market.NewBalance(map[string]any{
			"quote": "USD",
			"assets": []*market.Asset{
				market.NewAsset(nil, "USD", cash, shop.Zero, cash),
			},
		})

		fromDesk, deskErr := fixture.price.Allocate(fixture.symbol, nil)
		fromSmall, smallErr := fixture.price.Allocate(fixture.symbol, small)

		So(deskErr, ShouldBeNil)
		So(smallErr, ShouldBeNil)
		So(fromDesk, ShouldNotBeNil)
		So(fromSmall, ShouldNotBeNil)
		So(fromSmall.Cmp(*fromDesk), ShouldEqual, -1)
	})
}

func TestSpreadPctUsesBookSpread(t *testing.T) {
	Convey("SpreadPct is the book manager's relative bid-ask spread", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol:     "GGG/USD",
			cash:       1000,
			qtyMinRaw:  "1",
			costMinRaw: "1",
			step:       "1",
			bidPx:      99,
			bidQty:     100,
			askPx:      101,
			askQty:     100,
			feePct:     0.4,
		})

		spread, ok := fixture.price.SpreadPct(fixture.symbol)
		So(ok, ShouldBeTrue)
		So(spread, ShouldNotBeNil)
		// Book.Spread = (ask-bid)/ask*100 = 2/101*100
		So(spread.InexactFloat64(), ShouldAlmostEqual, 2.0/101.0*100.0, 1e-9)
	})
}

func TestPnLPctRoundTripExact(t *testing.T) {
	Convey("PnLPct is fee-net exit minus fee-net entry, over entry cost", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol:     "HHH/USD",
			cash:       1000,
			qtyMinRaw:  "1",
			costMinRaw: "1",
			step:       "1",
			bidPx:      100,
			bidQty:     100,
			askPx:      101,
			askQty:     100,
			feePct:     1.0,
		})

		qty := decimal.NewFromInt(10)
		entry := decimal.NewFromInt(100)
		fee := decimal.NewFromFloat(1.0)

		// cost = 1000 * 1.01 = 1010
		// sell through bid 100: gross exit 1000, net 990
		// pnl pct = (990-1010)/1010 * 100
		pct := fixture.price.PnLPct(fixture.symbol, &qty, &entry, &fee)
		So(pct, ShouldNotBeNil)
		So(pct.InexactFloat64(), ShouldAlmostEqual, (990.0-1010.0)/1010.0*100.0, 1e-9)
	})
}

func TestEntryCostMatchesAskWalk(t *testing.T) {
	Convey("EntryCost VWAP and fee match lifting the asks", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol:     "III/USD",
			cash:       1000,
			qtyMinRaw:  "1",
			costMinRaw: "1",
			step:       "1",
			bidPx:      99,
			bidQty:     100,
			askPx:      100,
			askQty:     100,
			feePct:     0.8,
		})

		qty := decimal.NewFromInt(2)
		fill, err := fixture.price.EntryCost(fixture.symbol, &qty)
		So(err, ShouldBeNil)
		So(fill.EntryPrice.String(), ShouldEqual, "100")
		So(fill.EntryFee.InexactFloat64(), ShouldAlmostEqual, 1.6, 1e-9)
	})
}

func TestExitRoomUsesBestBid(t *testing.T) {
	Convey("ExitRoom is best-bid quantity over lot size", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol:     "JJJ/USD",
			cash:       1000,
			qtyMinRaw:  "1",
			costMinRaw: "1",
			step:       "1",
			bidPx:      100,
			bidQty:     30,
			askPx:      101,
			askQty:     100,
			feePct:     0.4,
		})

		lot := decimal.NewFromInt(10)
		room := fixture.price.ExitRoom(fixture.symbol, &lot)
		So(room, ShouldNotBeNil)
		So(room.String(), ShouldEqual, "3")
	})
}

func TestTouchBidCashSumsTopThree(t *testing.T) {
	Convey("TouchBidCash sums quote notional on the top three bids", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol:     "KKK/USD",
			cash:       1000,
			qtyMinRaw:  "1",
			costMinRaw: "1",
			step:       "1",
			bidPx:      10,
			bidQty:     5,
			askPx:      11,
			askQty:     100,
			feePct:     0.4,
		})

		managed := fixture.books.Get(fixture.symbol)
		nine := decimal.NewFromInt(9)
		four := decimal.NewFromInt(4)
		eight := decimal.NewFromInt(8)
		three := decimal.NewFromInt(3)
		managed.Update(&market.Delta{Direction: market.SideBid, Price: &nine, Quantity: &four})
		managed.Update(&market.Delta{Direction: market.SideBid, Price: &eight, Quantity: &three})

		cash, ok := fixture.price.TouchBidCash(fixture.symbol)
		So(ok, ShouldBeTrue)
		// 10*5 + 9*4 + 8*3 = 50+36+24 = 110
		So(cash.InexactFloat64(), ShouldAlmostEqual, 110.0, 1e-9)
	})
}

type priceFixture struct {
	symbol     string
	cash       int64
	qtyMinRaw  string
	costMinRaw string
	step       string
	bidPx      int64
	bidQty     int64
	askPx      int64
	askQty     int64
	feePct     float64
	price      *Price
	wallet     *market.Balance
	books      *Book
	qtyMin     *decimal.Decimal
	costMin    *decimal.Decimal
}

func newPriceFixture(t *testing.T, cfg priceFixture) priceFixture {
	t.Helper()

	qtyMin, err := decimal.NewFromString(cfg.qtyMinRaw)
	if err != nil {
		t.Fatalf("qtyMin: %v", err)
	}

	costMin, err := decimal.NewFromString(cfg.costMinRaw)
	if err != nil {
		t.Fatalf("costMin: %v", err)
	}

	step, err := decimal.NewFromString(cfg.step)
	if err != nil {
		t.Fatalf("step: %v", err)
	}

	cash := shop.NewFromInt(cfg.cash)
	wallet := market.NewBalance(map[string]any{
		"quote": "USD",
		"assets": []*market.Asset{
			market.NewAsset(nil, "USD", cash, shop.Zero, cash),
		},
	})

	instrument := &Instrument{cache: &sync.Map{}, takerFees: &sync.Map{}}
	instrument.cache.Store(cfg.symbol, kraken.InstrumentPair{
		Symbol:         cfg.symbol,
		Quote:          "USD",
		QtyPrecision:   8,
		PricePrecision: 2,
		CostPrecision:  2,
		QtyMin:         &qtyMin,
		CostMin:        &costMin,
		QtyIncrement:   &step,
	})

	fee := decimal.NewFromFloat(cfg.feePct)
	instrument.takerFees.Store(cfg.symbol, &fee)

	books := NewBookManager()
	bidPx := decimal.NewFromInt(cfg.bidPx)
	bidQty := decimal.NewFromInt(cfg.bidQty)
	askPx := decimal.NewFromInt(cfg.askPx)
	askQty := decimal.NewFromInt(cfg.askQty)
	books.Update(&market.Level3{
		Data: []*market.Level3Data{{
			Symbol: cfg.symbol,
			Bids: []*market.Level3Order{{
				LimitPrice: &bidPx,
				OrderQty:   &bidQty,
			}},
			Asks: []*market.Level3Order{{
				LimitPrice: &askPx,
				OrderQty:   &askQty,
			}},
		}},
	})

	cfg.wallet = wallet
	cfg.books = books
	cfg.qtyMin = &qtyMin
	cfg.costMin = &costMin
	cfg.price = &Price{
		Instrument: instrument,
		Books:      books,
		balance:    wallet,
		normalizer: spot.NewNormalizer(),
	}

	return cfg
}
