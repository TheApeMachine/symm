package broker

import (
	"context"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/shopspring/decimal"
	shop "github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue"
	"github.com/theapemachine/symm/venue/kraken"
)

func TestAllocate(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("touch cap binds before cash", func() {
			fixture := newPriceFixture(t, priceFixture{
				symbol: "AAA/USD", cash: 1_000_000,
				qtyMinRaw: "0.01", costMinRaw: "1", step: "0.01",
				bidPx: 99, bidQty: 300, askPx: 100, askQty: 10_000, feePct: 0.4,
			})

			qty, err := fixture.price.Allocate(fixture.symbol, fixture.wallet)
			So(err, ShouldBeNil)
			So(qty.String(), ShouldEqual, "100")
		})

		Convey("fee-inclusive cost stays within max_fraction of cash", func() {
			fixture := newPriceFixture(t, priceFixture{
				symbol: "BBB/USD", cash: 1000,
				qtyMinRaw: "0.01", costMinRaw: "1", step: "0.01",
				bidPx: 99, bidQty: 100_000, askPx: 100, askQty: 100_000, feePct: 0.8,
			})

			qty, err := fixture.price.Allocate(fixture.symbol, fixture.wallet)
			So(err, ShouldBeNil)

			entryPx, entryFee, err := fixture.price.EntryCost(fixture.symbol, qty)
			So(err, ShouldBeNil)

			gross := entryPx.Mul(*qty)
			total := gross.Add(*entryFee)
			budget := decimal.NewFromFloat(200)

			So(total.Cmp(budget), ShouldBeLessThanOrEqualTo, 0)

			lo := decimal.RequireFromString("1.9")
			hi := decimal.RequireFromString("2.0")
			So(qty.Cmp(lo), ShouldBeGreaterThanOrEqualTo, 0)
			So(qty.Cmp(hi), ShouldBeLessThanOrEqualTo, 0)
		})

		Convey("refuses when touch cap cannot clear QtyMin", func() {
			fixture := newPriceFixture(t, priceFixture{
				symbol: "CCC/USD", cash: 100_000,
				qtyMinRaw: "50", costMinRaw: "1", step: "1",
				bidPx: 99, bidQty: 30, askPx: 100, askQty: 10_000, feePct: 0.4,
			})

			qty, err := fixture.price.Allocate(fixture.symbol, fixture.wallet)
			So(qty, ShouldBeNil)
			So(err.Error(), ShouldContainSubstring, "below venue minimum")
			So(err.Error(), ShouldContainSubstring, "touch_cap=10")
		})

		Convey("refuses when taker fee is unknown", func() {
			fixture := newPriceFixture(t, priceFixture{
				symbol: "NOFEE/USD", cash: 1_000_000,
				qtyMinRaw: "0.01", costMinRaw: "1", step: "0.01",
				bidPx: 99, bidQty: 300, askPx: 100, askQty: 10_000, feePct: 0.4,
			})
			fixture.price.Instrument.takerFees = &sync.Map{}

			qty, err := fixture.price.Allocate(fixture.symbol, fixture.wallet)
			So(qty, ShouldBeNil)
			So(err.Error(), ShouldContainSubstring, "taker fee unknown")
		})

		Convey("virtual learning lot bypasses venue QtyMin", func() {
			fixture := newPriceFixture(t, priceFixture{
				symbol: "DDD/USD", cash: 100_000,
				qtyMinRaw: "50", costMinRaw: "1", step: "1",
				bidPx: 99, bidQty: 30, askPx: 100, askQty: 10_000, feePct: 0.4,
			})

			venueQty, venueErr := fixture.price.Allocate(fixture.symbol, fixture.wallet)
			So(venueQty, ShouldBeNil)
			So(venueErr, ShouldNotBeNil)

			virtual, virtualErr := fixture.price.Allocate(fixture.symbol, nil)
			So(virtualErr, ShouldBeNil)
			So(virtual.String(), ShouldEqual, "10")
			So(virtual.Cmp(*fixture.qtyMin), ShouldEqual, -1)
		})

		Convey("sizes against the wallet argument not only desk balance", func() {
			fixture := newPriceFixture(t, priceFixture{
				symbol: "FFF/USD", cash: 100_000,
				qtyMinRaw: "0.01", costMinRaw: "1", step: "0.01",
				bidPx: 99, bidQty: 100_000, askPx: 100, askQty: 100_000, feePct: 0.4,
			})

			cash := shop.NewFromInt(500)
			small := market.NewBalance(map[string]any{
				"quote": "USD",
				"assets": []*market.Asset{
					market.NewAsset(nil, "USD", cash, shop.Zero, cash),
				},
			})

			fromDesk, _ := fixture.price.Allocate(fixture.symbol, nil)
			fromSmall, _ := fixture.price.Allocate(fixture.symbol, small)

			So(fromSmall.Cmp(*fromDesk), ShouldEqual, -1)
		})
	})
}

func TestLoadFeesRejectsEmptySymbols(t *testing.T) {
	Convey("loadFees refuses when the instrument list is empty", t, func() {
		price := &Price{
			System:     runtime.NewSystem(context.Background(), "price"),
			Instrument: &Instrument{symbols: []string{}},
		}

		So(price.loadFees(), ShouldBeFalse)
	})
}

func TestFeeNormalizerAlias(t *testing.T) {
	Convey("Fee resolves Kraken ledger aliases through the normalizer", t, func() {
		xrp := "XRP/USD"

		instrument := &Instrument{
			cache:     &sync.Map{},
			takerFees: &sync.Map{},
			symbols:   []string{xrp},
		}
		instrument.cache.Store(xrp, kraken.InstrumentPair{
			Symbol: xrp, Base: "XRP", Quote: "USD", Status: "online",
		})

		normalizer := spot.NewNormalizer()
		normalizer.Update(&spot.AssetsManagerUpdate{
			NewAssets: map[string]spot.AssetInfo{
				"XRP": {AltName: "XRP"},
				"USD": {AltName: "USD"},
			},
			OldAssets: map[string]spot.AssetInfo{
				"XXRP": {AltName: "XRP"},
				"ZUSD": {AltName: "USD"},
			},
			NewPairs: map[string]spot.AssetPair{
				"XRP/USD": {
					AltName: "XRPUSD", WSName: "XRP/USD", Base: "XRP", Quote: "USD",
				},
			},
			OldPairs: map[string]spot.AssetPair{
				"XXRPZUSD": {
					AltName: "XRPUSD", WSName: "XRP/USD", Base: "XXRP", Quote: "ZUSD",
				},
			},
		})

		price := &Price{Instrument: instrument, normalizer: normalizer}
		fee := decimal.NewFromFloat(0.26)

		symbol := price.normalizer.Name("XXRPZUSD")
		So(symbol, ShouldEqual, xrp)
		So(price.Instrument.Has(symbol), ShouldBeTrue)

		price.Instrument.takerFees.Store(symbol, &fee)
		So(price.Fee(xrp).String(), ShouldEqual, "0.26")
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

		// cost = 1000 * 1.01 = 1010
		// sell through bid 100: gross exit 1000, net 990
		// pnl pct = (990-1010)/1010 * 100
		pct := fixture.price.PnLPct(fixture.symbol, &qty, &entry)
		So(pct, ShouldNotBeNil)
		want := decimal.NewFromFloat((990.0 - 1010.0) / 1010.0 * 100.0)
		eps := decimal.RequireFromString("0.000000000000001")
		So(pct.Sub(want).Abs().LessThanOrEqual(eps), ShouldBeTrue)
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
		entryPx, entryFee, err := fixture.price.EntryCost(fixture.symbol, &qty)
		So(err, ShouldBeNil)
		So(entryPx.String(), ShouldEqual, "100")
		So(entryFee.Cmp(decimal.RequireFromString("1.6")), ShouldEqual, 0)
	})
}

func TestCloseNowPartialDepthMarksDown(t *testing.T) {
	Convey("thin bids mark only available depth; remainder is worthless", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol:     "THIN/USD",
			cash:       1000,
			qtyMinRaw:  "1",
			costMinRaw: "1",
			step:       "1",
			bidPx:      100,
			bidQty:     1,
			askPx:      101,
			askQty:     100,
			feePct:     0,
		})

		qty := decimal.NewFromInt(10)
		entry := decimal.NewFromInt(100)

		_, _, pnl, _ := fixture.price.MarkClose(fixture.symbol, &qty, &entry)
		So(pnl, ShouldNotBeNil)
		// Available exit 100; full entry cost 1000 → pnl -900.
		So(pnl.Cmp(decimal.RequireFromString("-900")), ShouldEqual, 0)
	})
}

func TestBackingRatioZeroAsksIsStrong(t *testing.T) {
	Convey("an ask vacuum inside the band is strongly backed", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol:     "BULL/USD",
			cash:       1000,
			qtyMinRaw:  "1",
			costMinRaw: "1",
			step:       "1",
			bidPx:      100,
			bidQty:     50,
			askPx:      101,
			askQty:     50,
			feePct:     0.4,
		})

		fixture.price.backingBand = 0.001

		// Ask just beyond the tiny band; Midpoint geometry leaves askCash at 0.
		far := decimal.RequireFromString("102")
		qty := decimal.NewFromInt(50)
		managed := fixture.books.Get(fixture.symbol)
		managed.Update(&market.Delta{
			Direction: market.SideAsk,
			Price:     managed.BestAsk().Price,
			Quantity:  &decimal.Zero,
		})
		managed.Update(&market.Delta{
			Direction: market.SideAsk,
			Price:     &far,
			Quantity:  &qty,
		})

		ratio, ok := fixture.price.BackingRatio(fixture.symbol)
		So(ok, ShouldBeTrue)
		So(math.IsInf(ratio, 1), ShouldBeTrue)
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
		So(cash.Cmp(decimal.RequireFromString("110")), ShouldEqual, 0)
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

	base, quote, _ := strings.Cut(cfg.symbol, "/")
	if quote == "" {
		quote = "USD"
	}

	instrument := &Instrument{cache: &sync.Map{}, takerFees: &sync.Map{}, quote: quote}
	instrument.cache.Store(cfg.symbol, kraken.InstrumentPair{
		Symbol:         cfg.symbol,
		Base:           base,
		Quote:          quote,
		QtyPrecision:   8,
		PricePrecision: 2,
		CostPrecision:  2,
		QtyMin:         &qtyMin,
		CostMin:        &costMin,
		QtyIncrement:   &step,
		TickSize:       &step,
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
		ticker:     market.NewTicker(nil),
		tape:       NewTape(),
		balance:    wallet,
		normalizer: NormalizerFor(instrument),
	}
	cfg.price.bindRisk()

	return cfg
}

func TestBelowPeak(t *testing.T) {
	Convey("BelowPeak respects max_entry_peak_frac", t, func() {
		price := &Price{ticker: market.NewTicker(nil), peakFrac: 0.90}

		Convey("It approves when last is under the cap", func() {
			price.SetTicker(&market.TickerData{
				Symbol: "Q/USD", VolumeUSD: 100_000, ChangePct: 2.0,
				High24h: 110, Low24h: 100, Last: 108,
			})
			So(price.BelowPeak("Q/USD"), ShouldBeTrue)

			frac, ok := price.PeakFrac("Q/USD")
			So(ok, ShouldBeTrue)
			So(frac, ShouldEqual, 0.8)
		})

		Convey("It vetoes when last is at or above the cap", func() {
			price.SetTicker(&market.TickerData{
				Symbol: "Q/USD", VolumeUSD: 100_000, ChangePct: 2.0,
				High24h: 110, Low24h: 100, Last: 109,
			})
			So(price.BelowPeak("Q/USD"), ShouldBeFalse)

			frac, ok := price.PeakFrac("Q/USD")
			So(ok, ShouldBeTrue)
			So(frac, ShouldEqual, 0.9)
		})

		Convey("It vetoes when range data is missing", func() {
			So(price.BelowPeak("MISSING/USD"), ShouldBeFalse)
		})
	})

	Convey("BelowPeak is disabled when the cap is out of (0,1)", t, func() {
		price := &Price{ticker: market.NewTicker(nil)}
		price.SetTicker(&market.TickerData{
			Symbol: "Q/USD", VolumeUSD: 100_000, ChangePct: 2.0,
			High24h: 110, Low24h: 100, Last: 110,
		})

		price.peakFrac = 0
		So(price.BelowPeak("Q/USD"), ShouldBeTrue)

		price.peakFrac = 1
		So(price.BelowPeak("Q/USD"), ShouldBeTrue)
	})
}

func TestApplyTickerViaVenue(t *testing.T) {
	Convey("applyTicker stores quote volume and peak range through the venue", t, func() {
		host := venue.NewKraken("USD")
		price := &Price{
			ticker:     market.NewTicker(nil),
			venue:      host,
			normalizer: spot.NewNormalizer(),
		}

		raw := []byte(`{
			"error": [],
			"result": {
				"AAA/USD": {
					"a": ["109","1","1.000"],
					"b": ["108","1","1.000"],
					"c": ["109","1"],
					"v": ["10","500"],
					"p": ["2","2"],
					"t": [1, 2],
					"l": ["100","100"],
					"h": ["110","110"],
					"o": "100"
				}
			}
		}`)

		So(host.Ticker().UnmarshalJSON(raw), ShouldBeNil)
		price.applyTicker(host.Ticker())

		So(price.VolumeUSD("AAA/USD"), ShouldEqual, 1000)
		frac, ok := price.PeakFrac("AAA/USD")
		So(ok, ShouldBeTrue)
		So(frac, ShouldEqual, 0.9)
	})
}

func TestSpreadFrictionExitRoom(t *testing.T) {
	Convey("spread, friction, and exit room follow the touch book", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol: "SPR/USD", cash: 1000,
			qtyMinRaw: "1", costMinRaw: "1", step: "1",
			bidPx: 100, bidQty: 4, askPx: 102, askQty: 100, feePct: 0.5,
		})

		spread, ok := fixture.price.SpreadPct(fixture.symbol)
		So(ok, ShouldBeTrue)
		// (ask-bid)/ask*100 for 100 vs 102
		wantSpread := decimal.RequireFromString("102").
			Sub(decimal.RequireFromString("100")).
			Div(decimal.RequireFromString("102")).
			Mul(decimal.RequireFromString("100"))
		eps := decimal.RequireFromString("0.0000001")
		So(spread.Sub(wantSpread).Abs().LessThanOrEqual(eps), ShouldBeTrue)

		friction, ok := fixture.price.Friction(fixture.symbol)
		So(ok, ShouldBeTrue)
		So(friction.Sign(), ShouldBeGreaterThan, 0)

		lot := decimal.NewFromInt(2)
		room := fixture.price.ExitRoom(fixture.symbol, &lot)
		So(room, ShouldNotBeNil)
		So(room.String(), ShouldEqual, "2")
	})
}

func TestBookIntact(t *testing.T) {
	Convey("BookIntact needs touch cash, backing, and accelerating tape", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol: "INT/USD", cash: 1000,
			qtyMinRaw: "1", costMinRaw: "1", step: "1",
			bidPx: 100, bidQty: 500, askPx: 101, askQty: 500, feePct: 0.4,
		})

		fixture.price.tape = NewTape()
		now := time.Now().UTC()
		window := system.Cfg.Risk.TapeWindow
		tapeFloor := system.Cfg.Risk.MinTapeNotionalUSD

		fixture.price.tape.Note(fixture.symbol, tapeFloor*2, now.Add(-window-time.Minute))
		fixture.price.tape.Note(fixture.symbol, tapeFloor/2, now.Add(-time.Minute))

		So(fixture.price.BookIntact(fixture.symbol), ShouldBeFalse)

		fixture.price.tape.Note(fixture.symbol, tapeFloor*2, now.Add(-time.Minute))

		So(fixture.price.BookIntact(fixture.symbol), ShouldBeTrue)
	})
}

func TestResolveDeskSymbol(t *testing.T) {
	Convey("ResolveDeskSymbol maps REST and compact pair names", t, func() {
		instrument := &Instrument{cache: &sync.Map{}, quote: "USD"}
		min := decimal.RequireFromString("0.01")
		instrument.cache.Store("XBT/USD", kraken.InstrumentPair{
			Symbol: "XBT/USD", Base: "XBT", Quote: "USD", Status: "online", QtyMin: &min,
		})

		price := &Price{Instrument: instrument, normalizer: spot.NewNormalizer()}

		So(price.ResolveDeskSymbol("XBT/USD"), ShouldEqual, "XBT/USD")
		So(price.ResolveDeskSymbol("XBTUSD"), ShouldEqual, "XBT/USD")
		So(price.ResolveDeskSymbol("XBT/EUR"), ShouldEqual, "")
	})
}

func TestFillAskAndTouchBid(t *testing.T) {
	Convey("FillAsk walks the ask; TouchBid reads the touch", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol: "FA/USD", cash: 1000,
			qtyMinRaw: "1", costMinRaw: "1", step: "1",
			bidPx: 99, bidQty: 5, askPx: 100, askQty: 3, feePct: 0,
		})

		qty := decimal.NewFromInt(2)
		ask := fixture.price.FillAsk(fixture.symbol, &qty)
		So(ask, ShouldNotBeNil)
		So(ask.String(), ShouldEqual, "100")

		bid := fixture.price.TouchBid(fixture.symbol)
		So(bid, ShouldNotBeNil)
		So(bid.String(), ShouldEqual, "99")
	})
}

func TestEntryCloseNowPnLPct(t *testing.T) {
	Convey("round-trip open drag is negative when spread and fees dominate", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol: "RT/USD", cash: 1000,
			qtyMinRaw: "1", costMinRaw: "1", step: "1",
			bidPx: 99, bidQty: 100, askPx: 101, askQty: 100, feePct: 0.5,
		})

		pct, ok := fixture.price.EntryCloseNowPnLPct(fixture.symbol)
		So(ok, ShouldBeTrue)
		So(pct, ShouldNotBeNil)
		So(pct.Sign(), ShouldBeLessThan, 0)
	})
}

func TestOpenDragCap(t *testing.T) {
	Convey("OpenDragCap never sits below twice the taker fee", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol: "CAP/USD", cash: 1000,
			qtyMinRaw: "1", costMinRaw: "1", step: "1",
			bidPx: 100, bidQty: 100, askPx: 101, askQty: 100, feePct: 0.8,
		})

		cap := fixture.price.OpenDragCap(fixture.symbol)
		So(cap, ShouldBeGreaterThanOrEqualTo, 1.6)
	})
}

func TestCloseMarkTickerFallback(t *testing.T) {
	Convey("CloseMark uses public last when the book is empty", t, func() {
		price := &Price{
			Books:  NewBookManager(),
			ticker: market.NewTicker(nil),
		}
		price.ticker.Set("BTC/USD", &market.TickerData{
			Symbol: "BTC/USD",
			Last:   42000.5,
		})

		mark := price.CloseMark("BTC/USD")

		So(mark, ShouldNotBeNil)
		So(mark.String(), ShouldEqual, "42000.5")
	})
}

func TestResolveDeskSymbolKrakenAliases(t *testing.T) {
	Convey("ResolveDeskSymbol matches Kraken compact asset aliases", t, func() {
		instrument := &Instrument{cache: &sync.Map{}, quote: "USD"}
		min := decimal.RequireFromString("0.01")
		instrument.cache.Store("ETH/USD", kraken.InstrumentPair{
			Symbol: "ETH/USD", Base: "XETH", Quote: "ZUSD", Status: "online", QtyMin: &min,
		})

		price := &Price{Instrument: instrument, normalizer: spot.NewNormalizer()}

		So(price.ResolveDeskSymbol("ETHUSD"), ShouldEqual, "ETH/USD")
		So(price.ResolveDeskSymbol("XETHZUSD"), ShouldEqual, "ETH/USD")
	})
}

func TestDeskSymbolForLedgerAsset(t *testing.T) {
	Convey("DeskSymbolForLedgerAsset maps compact Kraken asset codes", t, func() {
		instrument := &Instrument{cache: &sync.Map{}, quote: "USD"}
		min := decimal.RequireFromString("0.01")
		instrument.cache.Store("PEPE/USD", kraken.InstrumentPair{
			Symbol: "PEPE/USD", Base: "PEPE", Quote: "USD", Status: "online", QtyMin: &min,
		})

		price := &Price{Instrument: instrument, normalizer: spot.NewNormalizer()}

		So(price.DeskSymbolForLedgerAsset("USD"), ShouldEqual, "")
		So(price.DeskSymbolForLedgerAsset("PEPE"), ShouldEqual, "PEPE/USD")
		So(price.ResolveDeskSymbol("PEPEUSD"), ShouldEqual, "PEPE/USD")
	})
}

func TestFormatSize(t *testing.T) {
	Convey("formatSize rounds through the venue normalizer", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol: "FS/USD", cash: 1000,
			qtyMinRaw: "0.01", costMinRaw: "1", step: "0.01",
			bidPx: 10, bidQty: 100, askPx: 11, askQty: 100, feePct: 0.26,
		})

		raw := decimal.RequireFromString("1.234567")
		sized, err := fixture.price.formatSize(fixture.symbol, &raw)
		So(err, ShouldBeNil)
		So(sized, ShouldNotBeNil)
		So(sized.String(), ShouldEqual, "1.23")
	})
}

func TestMarkCloseHelpers(t *testing.T) {
	Convey("ExitVWAP, TouchAsk, and ticker passthrough follow the bid book", t, func() {
		fixture := newPriceFixture(t, priceFixture{
			symbol: "MC/USD", cash: 10_000,
			qtyMinRaw: "0.01", costMinRaw: "1", step: "0.01",
			bidPx: 40, bidQty: 10, askPx: 42, askQty: 10, feePct: 0.4,
		})

		qty := decimal.NewFromInt(2)
		vwap := fixture.price.ExitVWAP(fixture.symbol, &qty)
		So(vwap, ShouldNotBeNil)
		So(vwap.String(), ShouldEqual, "40")

		ask := fixture.price.TouchAsk(fixture.symbol)
		So(ask, ShouldNotBeNil)
		So(ask.String(), ShouldEqual, "42")

		fixture.price.SetTicker(&market.TickerData{
			Symbol: fixture.symbol, VolumeUSD: 1_000_000, ChangePct: -1.5,
			High24h: 50, Low24h: 30, Last: 41,
		})

		So(fixture.price.VolumeUSD(fixture.symbol), ShouldEqual, 1_000_000)
		So(fixture.price.ChangePct(fixture.symbol), ShouldEqual, -1.5)

		entry := decimal.NewFromInt(38)
		pct := fixture.price.PnLPct(fixture.symbol, &qty, &entry)
		So(pct, ShouldNotBeNil)
		So(pct.Sign(), ShouldBeGreaterThan, 0)
	})
}

func TestNewPrice(t *testing.T) {
	Convey("NewPrice loads fees and arms ticker watch", t, func() {
		prev := queryTradeVolume
		defer func() { queryTradeVolume = prev }()

		feeBody := []byte(`{
			"error":[],
			"result":{
				"fees":{"NP/USD":{"fee":"0.40"}}
			}
		}`)

		queryTradeVolume = func(url string, body any) ([]byte, error) {
			return feeBody, nil
		}

		min := decimal.RequireFromString("1")
		instrument := &Instrument{
			cache:     &sync.Map{},
			takerFees: &sync.Map{},
			symbols:   []string{"NP/USD"},
		}
		instrument.cache.Store("NP/USD", kraken.InstrumentPair{
			Symbol: "NP/USD", Base: "NP", Quote: "USD", Status: "online", QtyMin: &min,
		})

		watched := 0
		public := &tickerWatchClient{watches: &watched}

		host := venue.NewKraken("USD")
		price := NewPrice(
			context.Background(),
			NewBookManager(),
			instrument,
			spot.NewNormalizer(),
			host,
			public,
		)

		So(price, ShouldNotBeNil)
		So(price.Status(), ShouldEqual, runtime.READY)
		So(price.Fee("NP/USD").String(), ShouldEqual, "0.4")
		So(watched, ShouldEqual, 1)
	})

	Convey("kraken compact asset helpers map desk symbols", t, func() {
		instrument := &Instrument{cache: &sync.Map{}, quote: "USD"}
		min := decimal.RequireFromString("0.01")
		instrument.cache.Store("XBT/USD", kraken.InstrumentPair{
			Symbol: "XBT/USD", Base: "XXBT", Quote: "ZUSD", Status: "online", QtyMin: &min,
		})

		price := &Price{Instrument: instrument, normalizer: spot.NewNormalizer()}

		So(price.ResolveDeskSymbol("XXBTZUSD"), ShouldEqual, "XBT/USD")

		oneHalf := decimal.RequireFromString("1.5")
		out, err := toSDK(&oneHalf)
		So(err, ShouldBeNil)
		So(toSymm(out).String(), ShouldEqual, "1.5")
	})
}
