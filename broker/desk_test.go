package broker

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

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
		desk := NewDesk(ctx, &dummyTransport{}, nil, nil)
		So(desk, ShouldNotBeNil)

		Convey("When Enter is called with a symbol and quantity", func() {
			qty := decimal.NewFromFloat64(0.5)
			err := desk.Enter("BTC/USD", qty)
			So(err, ShouldBeNil)

			val, ok := desk.positions.Load("BTC/USD")
			So(ok, ShouldBeTrue)
			positions := val.([]*Position)
			So(len(positions), ShouldEqual, 1)

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
			rejected := NewDesk(ctx, transport, nil, nil)
			qty := decimal.NewFromFloat64(0.5)

			So(rejected.Enter("BTC/USD", qty), ShouldNotBeNil)
			So(rejected.Holds("BTC/USD"), ShouldBeFalse)

			transport.fail = false
			So(rejected.Enter("BTC/USD", qty), ShouldBeNil)
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
