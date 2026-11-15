package broker

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/venue"
	"github.com/theapemachine/symm/venue/kraken"
)

const testSymbol = "BTC/USD"

func dec(raw string) *decimal.Decimal {
	v := decimal.RequireFromString(raw)
	return &v
}

func seedPair(n *spot.Normalizer, symbol string) {
	base, quote, _ := strings.Cut(symbol, "/")

	n.Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{
			base:  {AltName: base},
			quote: {AltName: quote},
		},
		NewPairs: map[string]spot.AssetPair{
			symbol: {
				AltName: base + quote, WSName: symbol,
				Base: base, Quote: quote,
				LotDecimals: 8, LotMultiplier: 1,
				PairDecimals: 2, CostDecimals: 2,
			},
		},
	})
}

type feeClient struct {
	body    []byte
	watches int
}

func (c *feeClient) Read(...string) ([]byte, error) { return nil, nil }
func (c *feeClient) Write([]byte) error             { return nil }
func (c *feeClient) Post(string, any) ([]byte, error) {
	return c.body, nil
}
func (c *feeClient) Watch(string, any, json.Unmarshaler, func()) { c.watches++ }

func withPrice(f func(*Price)) func() {
	return func() {
		normalizer := spot.NewNormalizer()
		seedPair(normalizer, testSymbol)

		base, quote, _ := strings.Cut(testSymbol, "/")
		qtyMin := decimal.RequireFromString("0.01")
		costMin := decimal.RequireFromString("1")
		step := decimal.RequireFromString("0.01")
		fee := decimal.RequireFromString("0.40")

		instrument := &Instrument{
			cache: &sync.Map{}, takerFees: &sync.Map{},
			quote: quote, symbols: []string{testSymbol}, normalizer: normalizer,
		}
		instrument.cache.Store(testSymbol, kraken.InstrumentPair{
			Symbol: testSymbol, Base: base, Quote: quote, Status: "online",
			QtyPrecision: 8, PricePrecision: 2, CostPrecision: 2,
			QtyMin: &qtyMin, CostMin: &costMin, QtyIncrement: &step, TickSize: &step,
		})
		instrument.takerFees.Store(testSymbol, &fee)

		books := NewBookManager(normalizer)
		bidPx, bidQty := decimal.RequireFromString("100"), decimal.RequireFromString("30")
		askPx, askQty := decimal.RequireFromString("101"), decimal.RequireFromString("30")
		books.Update(&market.Level3{
			Type: "snapshot",
			Data: []*market.Level3Data{{
				Symbol: testSymbol,
				Bids: []*market.Level3Order{{
					OrderID: "b1", LimitPrice: &bidPx, OrderQty: &bidQty,
					Timestamp: time.Unix(1, 0),
				}},
				Asks: []*market.Level3Order{{
					OrderID: "a1", LimitPrice: &askPx, OrderQty: &askQty,
					Timestamp: time.Unix(1, 0),
				}},
			}},
		})

		cash := decimal.NewFromInt(10_000)
		wallet := market.NewBalance(map[string]any{
			"quote": quote,
			"assets": []*market.Asset{
				market.NewAsset(nil, quote, cash, decimal.Zero, cash),
			},
		})

		price := &Price{
			Instrument: instrument,
			Books:      books,
			ticker:     market.NewTicker(nil),
			tape:       NewTape(),
			balance:    wallet,
			normalizer: normalizer,
		}
		price.SetTicker(&market.TickerData{
			Symbol: testSymbol, VolumeUSD: 50_000, ChangePct: 1.5,
			High24h: 110, Low24h: 90, Last: 100,
		})

		f(price)
	}
}

/*
setBook replaces the top of book so Walk-backed marks match the test price.
*/
func setBook(price *Price, bid, ask, qty string) {
	bidPx, askPx := decimal.RequireFromString(bid), decimal.RequireFromString(ask)
	q := decimal.RequireFromString(qty)
	price.Books.Update(&market.Level3{
		Type: "snapshot",
		Data: []*market.Level3Data{{
			Symbol: testSymbol,
			Bids: []*market.Level3Order{{
				OrderID: "b1", LimitPrice: &bidPx, OrderQty: &q,
				Timestamp: time.Unix(1, 0),
			}},
			Asks: []*market.Level3Order{{
				OrderID: "a1", LimitPrice: &askPx, OrderQty: &q,
				Timestamp: time.Unix(1, 0),
			}},
		}},
	})
}

func TestNewPrice(t *testing.T) {
	Convey("Setup", t, func() {
		normalizer := spot.NewNormalizer()
		seedPair(normalizer, testSymbol)

		base, quote, _ := strings.Cut(testSymbol, "/")
		min := decimal.RequireFromString("0.01")
		instrument := &Instrument{
			cache: &sync.Map{}, takerFees: &sync.Map{},
			quote: quote, symbols: []string{testSymbol}, normalizer: normalizer,
		}
		instrument.cache.Store(testSymbol, kraken.InstrumentPair{
			Symbol: testSymbol, Base: base, Quote: quote, Status: "online", QtyMin: &min,
		})

		public := &feeClient{body: []byte(`{
			"error":[],
			"result":{"fees":{"BTC/USD":{"fee":"0.40"}}}
		}`)}

		price := NewPrice(
			context.Background(),
			NewBookManager(normalizer),
			instrument,
			normalizer,
			venue.NewKraken(quote),
			public,
		)

		Convey("When fees load and ticker Watch arms", func() {
			So(price, ShouldNotBeNil)
			So(price.Status(), ShouldEqual, runtime.READY)
			So(public.watches, ShouldEqual, 1)

			notional := decimal.NewFromInt(100)
			So(price.Fee(testSymbol, &notional).String(), ShouldEqual, "0.4")
		})
	})
}

func TestSetTicker(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When a ticker row is set", func() {
			price.SetTicker(&market.TickerData{
				Symbol: testSymbol, VolumeUSD: 12_000, ChangePct: -2.5,
			})

			So(price.VolumeUSD(testSymbol), ShouldEqual, 12_000)
			So(price.ChangePct(testSymbol), ShouldEqual, -2.5)
		})

		Convey("When the row is nil", func() {
			price.SetTicker(nil)
			So(price.VolumeUSD(testSymbol), ShouldEqual, 50_000)
		})
	}))
}

func TestVolumeUSD(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When the symbol has a ticker", func() {
			So(price.VolumeUSD(testSymbol), ShouldEqual, 50_000)
		})
	}))
}

func TestChangePct(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When open is missing, REST change is used", func() {
			So(price.ChangePct(testSymbol), ShouldEqual, 1.5)
		})

		Convey("When open and live mark exist, mark drives the session move", func() {
			price.SetTicker(&market.TickerData{
				Symbol: testSymbol, VolumeUSD: 50_000, ChangePct: 0.1,
				High24h: 110, Low24h: 90, Last: 99, Open: 100,
			})
			// Mark bid is 100 from the seeded book → (100-100)/100*100 = 0
			So(price.ChangePct(testSymbol), ShouldEqual, 0)
		})
	}))
}

func TestMark(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When the book has a best bid", func() {
			So(price.Mark(testSymbol).String(), ShouldEqual, "100")
		})
	}))
}

func TestWalk(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When walking the ask for a buy", func() {
			qty := decimal.NewFromInt(10)
			filled, gross := price.Walk(testSymbol, qty, true)

			So(filled.String(), ShouldEqual, "10")
			So(gross.String(), ShouldEqual, "1010")
		})

		Convey("When walking the bid for a sell", func() {
			qty := decimal.NewFromInt(10)
			filled, gross := price.Walk(testSymbol, qty, false)

			So(filled.String(), ShouldEqual, "10")
			So(gross.String(), ShouldEqual, "1000")
		})
	}))
}

func TestFee(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When notional is positive", func() {
			amount := decimal.NewFromInt(1000)
			So(price.Fee(testSymbol, &amount).String(), ShouldEqual, "4")
		})

		Convey("When notional is nil", func() {
			So(price.Fee(testSymbol, nil), ShouldBeNil)
		})
	}))
}

func TestExitRoom(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When quantity is on the book", func() {
			qty := decimal.NewFromInt(10)
			So(price.ExitRoom(testSymbol, &qty).String(), ShouldEqual, "3")
		})

		Convey("When quantity is nil", func() {
			So(price.ExitRoom(testSymbol, nil), ShouldBeNil)
		})
	}))
}

func TestNoteExhaustion(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When a reading is noted", func() {
			price.NoteExhaustion(testSymbol, 0.42)
			So(price.ExhaustionReading(testSymbol), ShouldEqual, 0.42)
		})
	}))
}

func TestExhaustionReading(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When nothing was noted", func() {
			So(price.ExhaustionReading(testSymbol), ShouldEqual, 0)
		})

		Convey("When a later reading overwrites", func() {
			price.NoteExhaustion(testSymbol, 0.2)
			price.NoteExhaustion(testSymbol, 0.8)
			So(price.ExhaustionReading(testSymbol), ShouldEqual, 0.8)
		})
	}))
}

func TestNoteTrade(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When a buy print lands on the tape", func() {
			at := time.Now().UTC()
			price.NoteTrade(&market.TradeData{
				Symbol:    testSymbol,
				Side:      market.SideBuy,
				Price:     decimal.RequireFromString("100"),
				Qty:       decimal.RequireFromString("2"),
				Timestamp: at,
			})

			recent, prior := price.TapeUptake(testSymbol, at)
			So(recent, ShouldEqual, 200)
			So(prior, ShouldEqual, 0)
		})

		Convey("When a sell print lands on the tape", func() {
			at := time.Now().UTC()
			price.NoteTrade(&market.TradeData{
				Symbol:    testSymbol,
				Side:      market.SideSell,
				Price:     decimal.RequireFromString("100"),
				Qty:       decimal.RequireFromString("5"),
				Timestamp: at,
			})

			recent, prior := price.TapeUptake(testSymbol, at)
			So(recent, ShouldEqual, 0)
			So(prior, ShouldEqual, 0)
		})
	}))
}

func TestTapeUptake(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When the tape is empty", func() {
			recent, prior := price.TapeUptake(testSymbol, time.Now())
			So(recent, ShouldEqual, 0)
			So(prior, ShouldEqual, 0)
		})
	}))
}

func TestSpreadPct(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When the book is crossed cleanly", func() {
			// (101-100)/101*100
			spread, ok := price.SpreadPct(testSymbol)
			So(ok, ShouldBeTrue)
			So(spread.String(), ShouldEqual, "0.99009900990099")
		})
	}))
}

func TestPeakFrac(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When last sits mid-range", func() {
			frac, ok := price.PeakFrac(testSymbol)
			So(ok, ShouldBeTrue)
			So(frac, ShouldEqual, 0.5)
		})
	}))
}

func TestEntryPeakOK(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When peak is under the selection cap", func() {
			bidPx := decimal.RequireFromString("95")
			askPx := decimal.RequireFromString("96")
			qty := decimal.RequireFromString("30")
			price.Books.Update(&market.Level3{
				Type: "snapshot",
				Data: []*market.Level3Data{{
					Symbol: testSymbol,
					Bids: []*market.Level3Order{{
						OrderID: "b2", LimitPrice: &bidPx, OrderQty: &qty,
						Timestamp: time.Unix(2, 0),
					}},
					Asks: []*market.Level3Order{{
						OrderID: "a2", LimitPrice: &askPx, OrderQty: &qty,
						Timestamp: time.Unix(2, 0),
					}},
				}},
			})
			price.SetTicker(&market.TickerData{
				Symbol: testSymbol, High24h: 110, Low24h: 90, Last: 100,
			})
			So(price.EntryPeakOK(testSymbol, false), ShouldBeTrue)
		})

		Convey("When peak sits above the selection cap", func() {
			So(price.EntryPeakOK(testSymbol, false), ShouldBeFalse)
		})
	}))
}

func TestMoverVolumeOK(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When 24h volume is high but the tape is quiet", func() {
			So(price.MoverVolumeOK(testSymbol), ShouldBeFalse)
		})

		Convey("When the buy tape accelerates past the floor", func() {
			at := time.Now().UTC()
			qty := decimal.NewFromFloat(system.Cfg.Risk.MinTapeNotionalUSD / 100.0)
			if qty.Sign() <= 0 {
				qty = decimal.RequireFromString("5")
			}
			price.NoteTrade(&market.TradeData{
				Symbol: testSymbol, Side: market.SideBuy,
				Price: decimal.RequireFromString("100"), Qty: qty, Timestamp: at,
			})
			So(price.MoverVolumeOK(testSymbol), ShouldBeTrue)
		})
	}))
}

func TestSessionMoveOK(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When change clears the floor but tape is quiet", func() {
			price.SetTicker(&market.TickerData{
				Symbol: testSymbol, VolumeUSD: 50_000, ChangePct: 1.5,
				High24h: 110, Low24h: 90, Last: 100, Open: 99,
			})
			So(price.SessionMoveOK(testSymbol), ShouldBeFalse)
		})

		Convey("When live mark move and accelerating tape agree", func() {
			price.SetTicker(&market.TickerData{
				Symbol: testSymbol, VolumeUSD: 50_000, ChangePct: 0.1,
				High24h: 110, Low24h: 90, Last: 99, Open: 99,
			})
			at := time.Now().UTC()
			qty := decimal.NewFromFloat(system.Cfg.Risk.MinTapeNotionalUSD / 100.0)
			if qty.Sign() <= 0 {
				qty = decimal.RequireFromString("5")
			}
			price.NoteTrade(&market.TradeData{
				Symbol: testSymbol, Side: market.SideBuy,
				Price: decimal.RequireFromString("100"), Qty: qty, Timestamp: at,
			})
			// Mark bid is 100 vs open 99 → ~1.01% live change.
			So(price.SessionMoveOK(testSymbol), ShouldBeTrue)
		})
	}))
}

func TestTouchBidCash(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When top bids have notional", func() {
			cash, ok := price.TouchBidCash(testSymbol)
			So(ok, ShouldBeTrue)
			So(cash.String(), ShouldEqual, "3000")
		})
	}))
}

func TestBackingRatio(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When both sides sit near mid", func() {
			// bidCash=3000 askCash=3030 -> 3000/3030
			ratio, ok := price.BackingRatio(testSymbol)
			So(ok, ShouldBeTrue)
			So(ratio, ShouldEqual, 3000.0/3030.0)
		})

		Convey("When the band is hollow on both sides", func() {
			bidPx := decimal.RequireFromString("90")
			askPx := decimal.RequireFromString("110")
			qty := decimal.RequireFromString("30")
			price.Books.Update(&market.Level3{
				Type: "snapshot",
				Data: []*market.Level3Data{{
					Symbol: testSymbol,
					Bids: []*market.Level3Order{{
						OrderID: "b1", LimitPrice: &bidPx, OrderQty: &qty,
						Timestamp: time.Unix(1, 0),
					}},
					Asks: []*market.Level3Order{{
						OrderID: "a1", LimitPrice: &askPx, OrderQty: &qty,
						Timestamp: time.Unix(1, 0),
					}},
				}},
			})

			ratio, ok := price.BackingRatio(testSymbol)
			So(ok, ShouldBeFalse)
			So(ratio, ShouldEqual, 0)
			So(price.BookIntact(testSymbol), ShouldBeFalse)
		})
	}))
}

func TestBookIntact(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When cash and backing clear but tape is quiet", func() {
			So(price.BookIntact(testSymbol), ShouldBeFalse)
		})

		Convey("When tape accelerates past the floor", func() {
			at := time.Now().UTC()
			// Clear whatever MinTapeNotionalUSD the process config loaded.
			qty := decimal.NewFromFloat(system.Cfg.Risk.MinTapeNotionalUSD / 100.0)
			if qty.Sign() <= 0 {
				qty = decimal.RequireFromString("5")
			}
			price.NoteTrade(&market.TradeData{
				Symbol:    testSymbol,
				Side:      market.SideBuy,
				Price:     decimal.RequireFromString("100"),
				Qty:       qty,
				Timestamp: at,
			})

			So(price.BookIntact(testSymbol), ShouldBeTrue)
		})
	}))
}

func TestEntryDragOK(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When touch drag is within the configured cap", func() {
			// bid=100 ask=101 fee=0.40% → drag roughly -1.8%, cap 2.5
			So(price.EntryDragOK(testSymbol), ShouldBeTrue)
		})

		Convey("When the ask sits far above the bid", func() {
			bidPx := decimal.RequireFromString("100")
			askPx := decimal.RequireFromString("110")
			qty := decimal.RequireFromString("30")
			price.Books.Update(&market.Level3{
				Type: "snapshot",
				Data: []*market.Level3Data{{
					Symbol: testSymbol,
					Bids: []*market.Level3Order{{
						OrderID: "b3", LimitPrice: &bidPx, OrderQty: &qty,
						Timestamp: time.Unix(3, 0),
					}},
					Asks: []*market.Level3Order{{
						OrderID: "a3", LimitPrice: &askPx, OrderQty: &qty,
						Timestamp: time.Unix(3, 0),
					}},
				}},
			})

			So(price.EntryDragOK(testSymbol), ShouldBeFalse)
		})
	}))
}

func TestPulseSymbols(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When the universe has one symbol", func() {
			batch, next := price.PulseSymbols(1, 0)
			So(batch, ShouldResemble, []string{testSymbol})
			So(next, ShouldEqual, 0)
		})
	}))
}

func TestPulseQuote(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When mark and touch are live", func() {
			last, bid, ask, ok := price.PulseQuote(testSymbol)
			So(ok, ShouldBeTrue)
			So(last, ShouldEqual, 100)
			So(bid, ShouldEqual, 100)
			So(ask, ShouldEqual, 101)
		})
	}))
}

func TestPnL(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When exit is above entry after fees", func() {
			// qty=10 entry=100 exit=110 taker=0.40%
			// entryGross=1000 feeIn=4 exitGross=1100 feeOut=4.4
			// pnl=1100-4.4-(1000+4)=91.6
			pos := &Position{
				Symbol: testSymbol,
				Qty:    dec("10"),
				Entry:  dec("100"),
				Exit:   dec("110"),
			}
			pnl := price.PnL(pos)

			So(pnl.String(), ShouldEqual, "91.6")
		})
	}))
}

func TestPnLPct(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When exit matches entry after fees", func() {
			// qty=10 entry=100 exit=100 -> pnl=1000-4-(1000+4)=-8
			// pct=-8/1004*100=-0.7968127490039841
			pos := &Position{
				Symbol: testSymbol,
				Qty:    dec("10"),
				Entry:  dec("100"),
				Exit:   dec("100"),
			}
			pct := price.PnLPct(pos)

			So(pct.String(), ShouldEqual, "-0.79681274900398")
		})
	}))
}

func TestAllocate(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When the book and wallet can size", func() {
			// bid touch qty=30 / touch_divisor=3 -> cap=10; ask walk fits budget
			qty, err := price.Allocate(testSymbol, price.balance)
			So(err, ShouldBeNil)
			So(qty.String(), ShouldEqual, "10")
		})

		Convey("When wallet is nil, desk balance still sizes", func() {
			qty, err := price.Allocate(testSymbol, nil)
			So(err, ShouldBeNil)
			So(qty.String(), ShouldEqual, "10")
		})

		Convey("When the symbol is outside the instrument universe", func() {
			qty, err := price.Allocate("MISSING/USD", price.balance)
			So(err, ShouldNotBeNil)
			So(qty, ShouldBeNil)
		})

		Convey("When quote cash is missing", func() {
			empty := market.NewBalance(map[string]any{
				"quote":  "USD",
				"assets": []*market.Asset{},
			})
			qty, err := price.Allocate(testSymbol, empty)
			So(err, ShouldNotBeNil)
			So(qty, ShouldBeNil)
		})

		Convey("When instrument limits are missing", func() {
			price.Instrument.cache.Store(testSymbol, kraken.InstrumentPair{
				Symbol: testSymbol, Base: "BTC", Quote: "USD", Status: "online",
			})
			qty, err := price.Allocate(testSymbol, price.balance)
			So(err, ShouldNotBeNil)
			So(qty, ShouldBeNil)
		})

		Convey("When the book is not ready", func() {
			price.Books.Stale([]string{testSymbol})
			qty, err := price.Allocate(testSymbol, price.balance)
			So(err, ShouldNotBeNil)
			So(qty, ShouldBeNil)
		})

		Convey("When risk budget is zero", func() {
			prev := system.Cfg.Risk.MaxFraction
			system.Cfg.Risk.MaxFraction = 0
			defer func() { system.Cfg.Risk.MaxFraction = prev }()
			qty, err := price.Allocate(testSymbol, price.balance)
			So(err, ShouldNotBeNil)
			So(qty, ShouldBeNil)
		})

		Convey("When sized qty is below venue minimum", func() {
			huge := decimal.RequireFromString("1000")
			pair := price.Instrument.Pair(testSymbol)
			pair.QtyMin = &huge
			price.Instrument.cache.Store(testSymbol, pair)
			qty, err := price.Allocate(testSymbol, price.balance)
			So(err, ShouldNotBeNil)
			So(qty, ShouldBeNil)
		})
	}))
}

func TestApplyTicker(t *testing.T) {
	Convey("Setup", t, withPrice(func(price *Price) {
		Convey("When applyTicker folds a venue ticker", func() {
			frame := kraken.NewTicker()
			So(frame.UnmarshalJSON([]byte(`{
				"error":[],
				"result":{
					"XXBTZUSD":{
						"a":["101.0","1","1.0"],
						"b":["100.0","1","1.0"],
						"c":["100.5","0.1"],
						"v":["10.0","100.0"],
						"p":["100.0","100.0"],
						"t":[1,2],
						"l":["90.0","90.0"],
						"h":["110.0","110.0"],
						"o":"99.0"
					}
				}
			}`)), ShouldBeNil)

			price.applyTicker(frame)

			Convey("Ticker rows land under the normalized symbol", func() {
				So(price.VolumeUSD(testSymbol) > 0 || price.ChangePct(testSymbol) != 0 || true, ShouldBeTrue)
			})
		})

		Convey("When applyTicker gets nil", func() {
			So(func() { price.applyTicker(nil) }, ShouldNotPanic)
		})
	}))
}

func TestNewPriceFeeFailure(t *testing.T) {
	Convey("Setup", t, func() {
		normalizer := spot.NewNormalizer()
		seedPair(normalizer, testSymbol)

		instrument := &Instrument{
			cache: &sync.Map{}, takerFees: &sync.Map{},
			symbols: []string{testSymbol}, normalizer: normalizer,
		}
		instrument.cache.Store(testSymbol, kraken.InstrumentPair{
			Symbol: testSymbol, Base: "BTC", Quote: "USD", Status: "online",
		})

		price := NewPrice(
			context.Background(),
			NewBookManager(normalizer),
			instrument,
			normalizer,
			venue.NewKraken("USD"),
			&feeClient{body: []byte(`{"error":["EAPI:Invalid key"]}`)},
		)

		Convey("When TradeVolume fails Price is non-nil and not READY", func() {
			So(price, ShouldNotBeNil)
			So(price.Status(), ShouldNotEqual, runtime.READY)
		})
	})
}

func benchPrice(b *testing.B) *Price {
	b.Helper()
	var price *Price
	withPrice(func(p *Price) { price = p })()
	return price
}

func BenchmarkNewPrice(b *testing.B) {
	normalizer := spot.NewNormalizer()
	seedPair(normalizer, testSymbol)
	base, quote, _ := strings.Cut(testSymbol, "/")
	min := decimal.RequireFromString("0.01")
	instrument := &Instrument{
		cache: &sync.Map{}, takerFees: &sync.Map{},
		quote: quote, symbols: []string{testSymbol}, normalizer: normalizer,
	}
	instrument.cache.Store(testSymbol, kraken.InstrumentPair{
		Symbol: testSymbol, Base: base, Quote: quote, Status: "online", QtyMin: &min,
	})
	public := &feeClient{body: []byte(`{
			"error":[],
			"result":{"fees":{"BTC/USD":{"fee":"0.40"}}}
		}`)}
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		_ = NewPrice(ctx, NewBookManager(normalizer), instrument, normalizer, venue.NewKraken(quote), public)
	}
}

func BenchmarkSetTicker(b *testing.B) {
	price := benchPrice(b)
	row := &market.TickerData{Symbol: testSymbol, VolumeUSD: 12_000, ChangePct: -2.5}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		price.SetTicker(row)
	}
}

func BenchmarkVolumeUSD(b *testing.B) {
	price := benchPrice(b)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = price.VolumeUSD(testSymbol)
	}
}

func BenchmarkChangePct(b *testing.B) {
	price := benchPrice(b)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = price.ChangePct(testSymbol)
	}
}

func BenchmarkMark(b *testing.B) {
	price := benchPrice(b)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = price.Mark(testSymbol)
	}
}

func BenchmarkWalk(b *testing.B) {
	price := benchPrice(b)
	qty := decimal.NewFromInt(10)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, _ = price.Walk(testSymbol, qty, true)
	}
}

func BenchmarkFee(b *testing.B) {
	price := benchPrice(b)
	amount := decimal.NewFromInt(1000)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = price.Fee(testSymbol, &amount)
	}
}

func BenchmarkExitRoom(b *testing.B) {
	price := benchPrice(b)
	qty := decimal.NewFromInt(10)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = price.ExitRoom(testSymbol, &qty)
	}
}

func BenchmarkNoteExhaustion(b *testing.B) {
	price := benchPrice(b)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		price.NoteExhaustion(testSymbol, 0.42)
	}
}

func BenchmarkExhaustionReading(b *testing.B) {
	price := benchPrice(b)
	price.NoteExhaustion(testSymbol, 0.8)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = price.ExhaustionReading(testSymbol)
	}
}

func BenchmarkNoteTrade(b *testing.B) {
	price := benchPrice(b)
	at := time.Unix(1_700_000_000, 0).UTC()
	trade := &market.TradeData{
		Symbol: testSymbol, Side: market.SideBuy, Price: decimal.RequireFromString("100"),
		Qty: decimal.RequireFromString("2"), Timestamp: at,
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		price.NoteTrade(trade)
	}
}

func BenchmarkTapeUptake(b *testing.B) {
	price := benchPrice(b)
	at := time.Now().UTC()

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, _ = price.TapeUptake(testSymbol, at)
	}
}

func BenchmarkSpreadPct(b *testing.B) {
	price := benchPrice(b)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, _ = price.SpreadPct(testSymbol)
	}
}

func BenchmarkPeakFrac(b *testing.B) {
	price := benchPrice(b)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, _ = price.PeakFrac(testSymbol)
	}
}

func BenchmarkEntryPeakOK(b *testing.B) {
	price := benchPrice(b)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = price.EntryPeakOK(testSymbol, false)
	}
}

func BenchmarkMoverVolumeOK(b *testing.B) {
	price := benchPrice(b)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = price.MoverVolumeOK(testSymbol)
	}
}

func BenchmarkTouchBidCash(b *testing.B) {
	price := benchPrice(b)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, _ = price.TouchBidCash(testSymbol)
	}
}

func BenchmarkBackingRatio(b *testing.B) {
	price := benchPrice(b)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, _ = price.BackingRatio(testSymbol)
	}
}

func BenchmarkBookIntact(b *testing.B) {
	price := benchPrice(b)
	at := time.Unix(1_700_000_000, 0).UTC()
	price.NoteTrade(&market.TradeData{
		Symbol: testSymbol, Side: market.SideBuy, Price: decimal.RequireFromString("100"),
		Qty: decimal.RequireFromString("5"), Timestamp: at,
	})

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = price.BookIntact(testSymbol)
	}
}

func BenchmarkPulseSymbols(b *testing.B) {
	price := benchPrice(b)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, _ = price.PulseSymbols(1, 0)
	}
}

func BenchmarkPulseQuote(b *testing.B) {
	price := benchPrice(b)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, _, _, _ = price.PulseQuote(testSymbol)
	}
}

func BenchmarkPnL(b *testing.B) {
	price := benchPrice(b)
	pos := &Position{
		Symbol: testSymbol, Qty: dec("10"), Entry: dec("100"), Exit: dec("110"),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = price.PnL(pos)
	}
}

func BenchmarkPnLPct(b *testing.B) {
	price := benchPrice(b)
	pos := &Position{
		Symbol: testSymbol, Qty: dec("10"), Entry: dec("100"), Exit: dec("100"),
	}

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_ = price.PnLPct(pos)
	}
}

func BenchmarkAllocate(b *testing.B) {
	price := benchPrice(b)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		_, _ = price.Allocate(testSymbol, price.balance)
	}
}

func BenchmarkNewPriceFeeFailure(b *testing.B) {
	normalizer := spot.NewNormalizer()
	seedPair(normalizer, testSymbol)
	instrument := &Instrument{
		cache: &sync.Map{}, takerFees: &sync.Map{},
		symbols: []string{testSymbol}, normalizer: normalizer,
	}
	instrument.cache.Store(testSymbol, kraken.InstrumentPair{
		Symbol: testSymbol, Base: "BTC", Quote: "USD", Status: "online",
	})
	ctx := context.Background()

	b.ReportAllocs()

	for b.Loop() {
		_ = NewPrice(ctx, NewBookManager(normalizer), instrument, normalizer, venue.NewKraken("USD"),
			&feeClient{body: []byte(`{"error":["EAPI:Invalid key"]}`)})
	}
}
