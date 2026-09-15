package broker

import (
	"sync"
	"testing"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/runtime"
	venue "github.com/theapemachine/symm/tests/venue"
)

/* newPriceSurface creates a price surface with the symbol's executable fee row. */
func newPriceSurface(t testing.TB, symbol string) (*Price, *websocket.API) {
	t.Helper()

	conn := venue.NewConn()
	api := websocket.NewAPI(t.Context(), conn, conn, &websocket.FuturesLive{})
	api.Transition(runtime.READY)
	price := newTestPrice(t, api)
	price.fees.Store(symbol, kraken.TradeVolumeFee{
		Fee: decimal.NewFromFloat64(0.25),
	})

	return price, api
}

/* newTestPrice builds a Price directly with an initialized instrument cache. */
func newTestPrice(t testing.TB, api *websocket.API) *Price {
	t.Helper()

	instrument := &Instrument{
		cache: &sync.Map{},
		quote: "USD",
	}

	return NewPrice(t.Context(), api, instrument)
}

/* newQuantityPrice creates the executable BTC/USD quantity fixture. */
func newQuantityPrice(t testing.TB) *Price {
	t.Helper()

	price, api := newPriceSurface(t, "BTC/USD")
	api.Normalizer().Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{
			"BTC": {AltName: "BTC", Decimals: 8, DisplayDecimals: 8},
			"USD": {AltName: "USD", Decimals: 2, DisplayDecimals: 2},
		},
		NewPairs: map[string]spot.AssetPair{
			"BTCUSD": {
				WSName: "BTC/USD", Base: "BTC", Quote: "USD",
				PairDecimals: 2, LotDecimals: 8, LotMultiplier: 1,
			},
		},
	})
	price.Instrument.cache.Store("BTC/USD", kraken.InstrumentPair{
		Symbol:       "BTC/USD",
		Base:         "BTC",
		Quote:        "USD",
		QtyMin:       decimal.NewFromFloat64(0.0001),
		CostMin:      decimal.NewFromFloat64(0.50),
		QtyIncrement: decimal.NewFromFloat64(0.00000001),
	})
	price.Update(&kraken.TickerData{
		Symbol: "BTC/USD",
		Ask:    decimal.NewFromFloat64(100000),
		Bid:    decimal.NewFromFloat64(99900),
	})

	return price
}

func TestPriceUpdate(t *testing.T) {
	Convey("Setup", t, func() {
		price, _ := newPriceSurface(t, "TEST1")

		Convey("Given some ticker data", func() {
			ticker := &kraken.TickerData{
				Symbol: "TEST1",
				Ask:    decimal.NewFromFloat64(30000.00),
				Bid:    decimal.NewFromFloat64(29950.00),
			}

			Convey("When the price surface is updated", func() {
				price.Update(ticker)

				Convey("It should store the new ticker data in the cache", func() {
					So(price.Tick("TEST1"), ShouldResemble, ticker)
				})
			})
		})
	})
}

func TestPriceMark(t *testing.T) {
	Convey("Setup", t, func() {
		price, _ := newPriceSurface(t, "TEST2")

		Convey("Given some ticker data", func() {
			ticker := &kraken.TickerData{
				Symbol: "TEST2",
				Ask:    decimal.NewFromFloat64(40000.00),
				Bid:    decimal.NewFromFloat64(39950.00),
			}

			price.Update(ticker)

			Convey("When the mark price is requested for buying", func() {
				markPrice := price.Mark("TEST2", BUY)

				Convey("It should return the ask with the taker fee", func() {
					So(markPrice.Float64(), ShouldAlmostEqual, 40100, 1e-12)
				})
			})

			Convey("When the mark price is requested for selling", func() {
				markPrice := price.Mark("TEST2", SELL)

				Convey("It should return the bid after the taker fee", func() {
					So(markPrice.Float64(), ShouldAlmostEqual, 39850.125, 1e-12)
				})
			})
		})
	})
}

func TestPricePnL(t *testing.T) {
	Convey("Setup", t, func() {
		price, _ := newPriceSurface(t, "TEST3")
		position := &Position{
			EntryOrder: &spot.AddOrderRequest{
				Pair:   "TEST3",
				Volume: "1.0",
				Price:  "45000.00",
			},
		}

		Convey("Given an authoritative economic mark", func() {
			price.Update(&kraken.TickerData{
				Symbol: "TEST3",
				Bid:    decimal.NewFromFloat64(49950.00),
				Ask:    decimal.NewFromFloat64(50050.00),
			})

			Convey("When the PnL is calculated for a position", func() {
				pnl := price.PnL("TEST3", position)

				Convey("It should return the profit or loss based on the authoritative mark, including fees", func() {
					So(pnl, ShouldNotBeNil)
					So(pnl.Float64(), ShouldAlmostEqual, 4825.125, 1e-12)
				})
			})
		})
	})

	Convey("Given a position before its mark is set", t, func() {
		price, _ := newPriceSurface(t, "COLD/USD")
		position := &Position{
			EntryOrder: &spot.AddOrderRequest{
				Pair:   "COLD/USD",
				Volume: "1.0",
				Price:  "100",
			},
		}

		Convey("It should reject the incomplete valuation without dereferencing it", func() {
			So(price.PnL("COLD/USD", position), ShouldBeNil)
		})
	})
}

func TestPriceExitValue(t *testing.T) {
	Convey("Setup", t, func() {
		price, _ := newPriceSurface(t, "TEST4")
		position := &Position{
			EntryOrder: &spot.AddOrderRequest{
				Pair:   "TEST4",
				Volume: "2.0",
				Price:  "60000.00",
			},
		}

		Convey("Given an authoritative economic mark", func() {
			price.Update(&kraken.TickerData{
				Symbol: "TEST4",
				Bid:    decimal.NewFromFloat64(64950.00),
				Ask:    decimal.NewFromFloat64(65050.00),
			})

			Convey("When the exit value is calculated for a position", func() {
				exitValue := price.ExitValue("TEST4", position)

				Convey("It should return the exit value based on the authoritative mark, fee-net", func() {
					So(exitValue, ShouldNotBeNil)
					So(exitValue.Float64(), ShouldAlmostEqual, 129575.25, 1e-12)
				})
			})
		})
	})
}

func TestPriceTick(t *testing.T) {
	Convey("Setup", t, func() {
		price, _ := newPriceSurface(t, "TEST7")

		Convey("Given some ticker data", func() {
			ticker := &kraken.TickerData{
				Symbol: "TEST7",
				Ask:    decimal.NewFromFloat64(80000.00),
				Bid:    decimal.NewFromFloat64(79950.00),
			}

			price.Update(ticker)

			Convey("When the tick is requested for a symbol", func() {
				tick := price.Tick("TEST7")

				Convey("It should return the latest ticker data for that symbol", func() {
					So(tick, ShouldResemble, ticker)
				})
			})
		})
	})
}

func TestPriceTradable(t *testing.T) {
	Convey("Given instrument constraints on BTC/USD", t, func() {
		price := newQuantityPrice(t)

		Convey("Quantities below minimum or notional below minimum are not tradable", func() {
			unit := decimal.NewFromInt64(100000)
			So(price.Tradable("BTC/USD", decimal.NewFromFloat64(0.00001), unit), ShouldBeFalse)
			So(price.Tradable("BTC/USD", decimal.NewFromFloat64(0.001), unit), ShouldBeTrue)
		})
	})
}


func TestPriceGetFees(t *testing.T) {
	Convey("Given a mock API responding with trade volume fee data", t, func() {
		price, _ := newPriceSurface(t, "BTC/USD")
		conn := venue.NewConn()
		conn.TradeVolumeResult = &kraken.TradeVolumeResult{
			Fees: map[string]kraken.TradeVolumeFee{
				"XXBTZUSD": {Fee: decimal.NewFromFloat64(0.26)},
			},
		}
		api := websocket.NewAPI(t.Context(), conn, conn, &websocket.FuturesLive{})
		api.Transition(runtime.READY)
		api.Normalizer().Update(&spot.AssetsManagerUpdate{
			NewAssets: map[string]spot.AssetInfo{
				"BTC": {AltName: "BTC"},
				"USD": {AltName: "USD"},
			},
			OldAssets: map[string]spot.AssetInfo{
				"XXBT": {AltName: "BTC"},
				"ZUSD": {AltName: "USD"},
			},
			NewPairs: map[string]spot.AssetPair{
				"XXBTZUSD": {WSName: "BTC/USD", Base: "XXBT", Quote: "ZUSD"},
			},
		})
		price.api = api
		price.normalizer = api.Normalizer()

		Convey("GetFees normalizes the key once and records it under canonical symbol", func() {
			err := price.GetFees([]string{"BTC/USD"})
			So(err, ShouldBeNil)
			So(price.Status(), ShouldEqual, runtime.READY)
			fee := price.Fee("BTC/USD")
			So(fee, ShouldNotBeNil)
			So(fee.Fee.Float64(), ShouldAlmostEqual, 0.26, 1e-12)
		})
	})
}

/*
A venue quoting both sides from instants that disagree produces a touch that
meets or crosses. It prices no entry and no liquidation, but it is a reading
about book shape rather than a fault, so callers must be able to size around it
the way they size around insufficient depth.
*/
func TestPriceCrossedBookIsAReading(t *testing.T) {
	Convey("A crossed touch is unprocessable, not a validation failure", t, func() {
		touch := &touchBook{}
		price := newQuantityPrice(t)
		price.Books = touch
		touch.quote("BTC/USD", 100000, 99900)
		quantity := decimal.NewFromFloat64(0.001)

		cost, err := price.EntryCost("BTC/USD", quantity)
		So(cost, ShouldBeNil)
		So(errnie.IsUnprocessableContent(err), ShouldBeTrue)
		So(errnie.IsValidation(err), ShouldBeFalse)

		surface, err := price.Surface("BTC/USD", quantity, time.Now().UTC())
		So(surface.FullyExecutable, ShouldBeFalse)
		So(errnie.IsUnprocessableContent(err), ShouldBeTrue)
		So(errnie.IsValidation(err), ShouldBeFalse)

		time.Sleep(15 * time.Millisecond)
		So(price.Anomalies().Count("BTC/USD"), ShouldBeGreaterThan, 0)
		So(price.MarketHealth("BTC/USD"), ShouldBeLessThan, 1.0)

		Convey("A touch quoted with no spread at all reads the same way", func() {
			touch.quote("BTC/USD", 100000, 100000)
			_, err := price.EntryCost("BTC/USD", quantity)
			So(errnie.IsUnprocessableContent(err), ShouldBeTrue)
		})

		Convey("An ordinary touch still prices", func() {
			touch.quote("BTC/USD", 99900, 100000)
			cost, err := price.EntryCost("BTC/USD", quantity)
			So(err, ShouldBeNil)
			So(cost.Total.Sign(), ShouldEqual, 1)
		})
	})
}

// touchBook exposes one quoted touch, the way a captured observation does.
type touchBook struct{ current *spotbook.Book }

func (source *touchBook) quote(symbol string, bid, ask float64) {
	source.current = &spotbook.Book{
		Name: symbol,
		Bids: &spotbook.Side{High: &spotbook.Level{
			Price: decimal.NewFromFloat64(bid), Quantity: decimal.NewFromFloat64(1),
		}},
		Asks: &spotbook.Side{Low: &spotbook.Level{
			Price: decimal.NewFromFloat64(ask), Quantity: decimal.NewFromFloat64(1),
		}},
	}
}

func (source *touchBook) Book(symbol string, read func(*spotbook.Book)) {
	if source.current != nil && source.current.Name == symbol {
		read(source.current)
	}
}


