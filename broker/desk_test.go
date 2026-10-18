package broker

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/system"
)

func init() {
	if system.Cfg != nil && system.Cfg.Risk != nil {
		system.Cfg.Risk.MaxFraction = decimal.NewFromFloat64(0.20)
	}
}

func testNormalizer() *spot.Normalizer {
	n := spot.NewNormalizer()
	n.Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{
			"BTC": {AltName: "XBT"},
			"ETH": {AltName: "ETH"},
			"USD": {AltName: "USD"},
		},
		NewPairs: map[string]spot.AssetPair{
			"BTC/USD": {
				WSName: "BTC/USD", Base: "BTC", Quote: "USD",
				LotDecimals: 8, LotMultiplier: 1,
			},
			"ETH/USD": {
				WSName: "ETH/USD", Base: "ETH", Quote: "USD",
				LotDecimals: 8, LotMultiplier: 1,
			},
		},
	})
	return n
}

func deskWithQuote(
	ctx context.Context,
	transport Transport,
	symbol string,
	ask *decimal.Decimal,
	cash float64,
) *Desk {
	price := NewPrice(ctx, nil, nil, nil, testNormalizer())
	fee := decimal.NewFromFloat64(0.26)
	price.SetFee(symbol, kraken.TradeVolumeFee{Fee: fee, Minfee: fee, Maxfee: fee})
	bid := decimal.NewFromFloat64(ask.Float64() * 0.99)
	price.SetQuote(symbol, bid, ask)
	balance := &Balance{Quote: "USD"}
	balance.wallet.Store(&kraken.Balance{
		Data: []kraken.BalanceData{{
			Asset:   "USD",
			Balance: decimal.NewFromFloat64(cash),
		}},
	})
	return NewDesk(ctx, transport, price, balance)
}

type dummyTransport struct{}

func (d *dummyTransport) Read(keys ...string) ([]byte, error) {
	return nil, nil
}

func (d *dummyTransport) Write([]byte) error {
	return nil
}

type failTransport struct {
	writes int
	fail   bool
}

func (transport *failTransport) Read(keys ...string) ([]byte, error) {
	return nil, nil
}

func (transport *failTransport) Write([]byte) error {
	if transport.fail {
		return errors.New("venue down")
	}

	transport.writes++
	return nil
}

func TestDesk(t *testing.T) {
	Convey("Given a Desk instance", t, func() {
		ctx := context.Background()
		desk := deskWithQuote(
			ctx, &dummyTransport{}, "BTC/USD", decimal.NewFromFloat64(50_000), 10_000,
		)
		So(desk, ShouldNotBeNil)

		Convey("When Enter is called for a symbol", func() {
			err := desk.Enter("BTC/USD")
			So(err, ShouldBeNil)

			val, ok := desk.positions.Load("BTC/USD")
			So(ok, ShouldBeTrue)
			positions := val.([]*Position)
			So(len(positions), ShouldEqual, 1)
			qty := positions[0].Qty()
			So(qty, ShouldNotBeNil)

			Convey("When Apply is called with an execution", func() {
				desk.Apply(&kraken.Execution{
					Data: []kraken.ExecutionData{
						{
							Symbol:   "BTC/USD",
							ExecType: "trade",
							LastQty:  qty,
						},
					},
				})

				Convey("When Exit is called", func() {
					err := desk.Exit("BTC/USD")
					So(err, ShouldBeNil)
				})
			})
		})

		Convey("A rejected order does not leave the symbol held or exiting", func() {
			transport := &failTransport{fail: true}
			rejected := deskWithQuote(
				ctx, transport, "BTC/USD", decimal.NewFromFloat64(50_000), 10_000,
			)

			So(rejected.Enter("BTC/USD"), ShouldNotBeNil)
			So(rejected.Holds("BTC/USD"), ShouldBeFalse)

			transport.fail = false
			So(rejected.Enter("BTC/USD"), ShouldBeNil)
			So(rejected.Holds("BTC/USD"), ShouldBeTrue)
			So(transport.writes, ShouldEqual, 1)

			transport.fail = true
			So(rejected.Exit("BTC/USD"), ShouldNotBeNil)

			transport.fail = false
			So(rejected.Exit("BTC/USD"), ShouldBeNil)
			So(transport.writes, ShouldEqual, 2)
		})

		Convey("When initialized with pre-existing wallet holding", func() {
			balance := &Balance{Quote: "USD"}
			balance.wallet.Store(&kraken.Balance{
				Data: []kraken.BalanceData{
					{
						Asset:   "USD",
						Balance: decimal.NewFromFloat64(160.14),
					},
					{
						Asset:   "ZAMA",
						Balance: decimal.NewFromFloat64(517.71),
					},
				},
			})

			restoredDesk := NewDesk(ctx, &dummyTransport{}, nil, balance)
			So(restoredDesk.Holds("ZAMA/USD"), ShouldBeTrue)

			val, ok := restoredDesk.positions.Load("ZAMA/USD")
			So(ok, ShouldBeTrue)
			posList := val.([]*Position)
			So(len(posList), ShouldEqual, 1)
			So(posList[0].Qty().String(), ShouldEqual, "517.71")
			So(posList[0].Closed(), ShouldBeFalse)
		})
	})
}

func TestPositionsJSONReadsLotSlices(t *testing.T) {
	desk := &Desk{positions: &sync.Map{}}
	position := NewPosition()
	position.Enter("RLC/USD", decimal.NewFromFloat64(2))
	desk.positions.Store("RLC/USD", []*Position{position})

	raw := string(desk.PositionsJSON())

	if !strings.Contains(raw, `"symbol":"RLC/USD"`) {
		t.Fatalf("positions layer = %s", raw)
	}
}

func TestBalanceJSONCarriesTradeValuation(t *testing.T) {
	balance := &Balance{}
	balance.wallet.Store(&kraken.Balance{
		Data: []kraken.BalanceData{{
			Asset:   "USD",
			Balance: decimal.NewFromFloat64(120),
		}},
	})
	balance.trade.Store(&kraken.TradeBalanceResult{
		UnrealizedPnL: decimal.NewFromFloat64(-4.5),
		Equity:        decimal.NewFromFloat64(195.5),
	})

	raw := string((&Desk{balance: balance}).BalanceJSON())

	if !strings.Contains(raw, `"unrealized":"-4.5"`) || !strings.Contains(raw, `"equity":"195.5"`) {
		t.Fatalf("balance layer = %s", raw)
	}
}

type captureTransport struct {
	body []byte
}

func (transport *captureTransport) Read(keys ...string) ([]byte, error) {
	return nil, nil
}

func (transport *captureTransport) Write(buf []byte) error {
	transport.body = append([]byte(nil), buf...)
	return nil
}

func TestClosedLotIsKept(t *testing.T) {
	desk := &Desk{positions: &sync.Map{}}
	position := NewPosition()
	position.Enter("TEL/USD", decimal.NewFromFloat64(2))
	desk.positions.Store("TEL/USD", []*Position{position})

	desk.Apply(&kraken.Execution{Data: []kraken.ExecutionData{{
		Symbol:    "TEL/USD",
		Side:      "sell",
		ExecType:  "trade",
		LastQty:   decimal.NewFromFloat64(2),
		LastPrice: decimal.NewFromFloat64(11),
	}}})

	closed := desk.Closed()

	if len(closed) != 1 || closed[0].ID() != position.ID() || !closed[0].Closed() {
		t.Fatalf("closed %+v", closed)
	}

	if len(desk.OpenPositions()) != 0 {
		t.Fatal("closed lot still open")
	}
}

func TestRestoreStoredLotCanBeSold(t *testing.T) {
	transport := &captureTransport{}
	desk := &Desk{positions: &sync.Map{}, transport: transport}
	opened := time.Now().UTC()
	raw, err := (&Position{
		symbol:     "RLC/USD",
		qty:        decimal.NewFromFloat64(12.5),
		entryPrice: decimal.NewFromFloat64(1.2),
		openedAt:   opened,
	}).MarshalJSON()

	if err != nil {
		t.Fatal(err)
	}

	symbol, err := desk.RestoreStored(raw)

	if err != nil {
		t.Fatal(err)
	}

	if symbol != "RLC/USD" || !desk.Holds("RLC/USD") {
		t.Fatalf("restored %q holds=%v", symbol, desk.Holds("RLC/USD"))
	}

	if !strings.Contains(string(desk.PositionsJSON()), `"symbol":"RLC/USD"`) {
		t.Fatalf("positions layer = %s", desk.PositionsJSON())
	}

	if err := desk.Exit("RLC/USD"); err != nil {
		t.Fatal(err)
	}

	sold := string(transport.body)

	if !strings.Contains(sold, `"side":"sell"`) || !strings.Contains(sold, `"order_qty":"12.5"`) {
		t.Fatalf("exit order = %s", sold)
	}

	flat, err := (&Position{symbol: "RLC/USD"}).MarshalJSON()

	if err != nil {
		t.Fatal(err)
	}

	symbol, err = desk.RestoreStored(flat)

	if err != nil || symbol != "" {
		t.Fatalf("flat lot restored as %q (%v)", symbol, err)
	}
}

type fillOnWrite struct {
	desk *Desk
	fill *kraken.Execution
}

func (transport *fillOnWrite) Read(...string) ([]byte, error) { return nil, nil }

func (transport *fillOnWrite) Write([]byte) error {
	if transport.desk != nil && transport.fill != nil {
		transport.desk.Apply(transport.fill)
	}

	return nil
}

func TestBuyFillDuringSubmitIsKept(t *testing.T) {
	qty := decimal.NewFromFloat64(0.00840739)
	paid := decimal.NewFromFloat64(2500)
	transport := &fillOnWrite{fill: &kraken.Execution{Data: []kraken.ExecutionData{{
		Symbol: "ETH/USD", Side: "buy", ExecType: "trade",
		LastQty: qty, LastPrice: paid, AvgPrice: paid,
	}}}}
	ctx := context.Background()
	desk := deskWithQuote(ctx, transport, "ETH/USD", paid, 10_000)
	transport.desk = desk

	if err := desk.Enter("ETH/USD"); err != nil {
		t.Fatal(err)
	}

	held := desk.Open("ETH/USD")

	if len(held) != 1 || held[0].EntryPrice() == nil || held[0].EntryPrice().Cmp(paid) != 0 {
		got := "none"
		if len(held) == 1 && held[0].EntryPrice() != nil {
			got = held[0].EntryPrice().String()
		}
		t.Fatalf("entry = %s, want the buy fill", got)
	}
}

func TestSaleWithoutAPurchaseIsNotAProfit(t *testing.T) {
	qty := decimal.NewFromFloat64(0.00840739)
	position := NewPosition()
	position.Enter("ETH/USD", qty)
	position.Exit("ETH/USD")
	position.Apply(kraken.ExecutionData{
		Symbol: "ETH/USD", Side: "sell", ExecType: "trade",
		LastQty: qty, LastPrice: decimal.NewFromFloat64(2418.69),
		FeeUsdEquiv: decimal.NewFromFloat64(0.1627),
	})

	life := position.Lifecycle()

	if life.Outcome != nil && life.Outcome.VenueRealized > 0 {
		t.Fatalf("realized %v from a sale with no purchase", life.Outcome.VenueRealized)
	}
}

func TestCloseBelowTheEntryIsALoss(t *testing.T) {
	ctx := context.Background()
	symbol := "ETH/USD"
	desk := deskWithQuote(ctx, &dummyTransport{}, symbol, decimal.NewFromFloat64(101), 500)

	if err := desk.Enter(symbol); err != nil {
		t.Fatal(err)
	}

	held := desk.Open(symbol)

	if len(held) != 1 || held[0].Qty() == nil {
		t.Fatal("enter did not open a lot")
	}

	qty := held[0].Qty()

	if err := desk.Exit(symbol); err != nil {
		t.Fatal(err)
	}

	desk.Apply(&kraken.Execution{Data: []kraken.ExecutionData{{
		Symbol: symbol, Side: "sell", ExecType: "trade",
		LastQty: qty, LastPrice: decimal.NewFromFloat64(2418.69),
	}}})

	closed := desk.Closed()

	if len(closed) != 1 || closed[0].Lifecycle().Outcome == nil {
		t.Fatal("close was not scored")
	}

	if closed[0].Lifecycle().Outcome.VenueRealized >= 0 {
		t.Fatalf("realized %v, want a loss from the book", closed[0].Lifecycle().Outcome.VenueRealized)
	}
}

func TestEnterKeepsTheBookPriceUntilAFillNamesOne(t *testing.T) {
	ctx := context.Background()
	paid := decimal.NewFromFloat64(100)
	price := NewPrice(ctx, nil, nil, nil, testNormalizer())
	fee := decimal.NewFromFloat64(0.26)
	price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: fee, Minfee: fee, Maxfee: fee})
	price.SetQuote("BTC/USD", paid, paid)
	balance := &Balance{Quote: "USD"}
	balance.wallet.Store(&kraken.Balance{
		Data: []kraken.BalanceData{{
			Asset:   "USD",
			Balance: decimal.NewFromFloat64(10_000),
		}},
	})
	desk := NewDesk(ctx, &dummyTransport{}, price, balance)

	if err := desk.Enter("BTC/USD"); err != nil {
		t.Fatal(err)
	}

	held := desk.Open("BTC/USD")

	if len(held) != 1 || held[0].EntryPrice() == nil || held[0].EntryPrice().Cmp(paid) != 0 {
		t.Fatal("the order left the desk with no entry price")
	}

	desk.Apply(&kraken.Execution{Data: []kraken.ExecutionData{{
		Symbol:    "BTC/USD",
		Side:      "buy",
		ExecType:  "trade",
		LastQty:   decimal.NewFromFloat64(0.01),
		LastPrice: decimal.NewFromFloat64(0),
		AvgPrice:  decimal.NewFromFloat64(0),
	}}})

	if held[0].EntryPrice().Cmp(paid) != 0 {
		t.Fatal("a fill with no price cleared the entry")
	}

	desk.Apply(&kraken.Execution{Data: []kraken.ExecutionData{{
		Symbol:    "BTC/USD",
		Side:      "buy",
		ExecType:  "trade",
		LastQty:   decimal.NewFromFloat64(0.01),
		LastPrice: decimal.NewFromFloat64(101),
	}}})

	if held[0].EntryPrice().Float64() != 101 {
		t.Fatalf("entry = %s, want the fill", held[0].EntryPrice())
	}
}
