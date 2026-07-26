package broker

import (
	"context"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/types"
)

/*
TestDeskBalanceAdopt proves restart-visible wallet lots become Desk positions
without submitting another entry order. The balance snapshot is the paper/live
state truth; executions only supply entry economics for the adopted lot.
*/
func TestDeskBalanceAdopt(t *testing.T) {
	Convey("Given a desk with instrument rules and a wallet-backed paper lot", t, func() {
		public := mockapi.NewConn(t.Context())
		private := mockapi.NewConn(t.Context())
		api := websocket.NewAPI(t.Context(), public, private, nil)
		So(api.Initialize(), ShouldBeNil)
		price := NewPrice(api)
		So(price.RememberFee("BTC/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.1),
		}), ShouldBeNil)
		instrument := NewInstrument(api, price, make(chan []byte, 4), config.Fixture().Market)
		instrument.Remember(kraken.InstrumentPair{
			Symbol:        "BTC/USD",
			Base:          "BTC",
			Quote:         "USD",
			Status:        "online",
			CostPrecision: 4,
			QtyIncrement:  decimal.NewFromFloat64(0.00000001),
		})
		balance := NewBalance(api, nil, make(chan []byte, 4), config.Fixture().Market)
		desk := NewDesk(
			context.Background(),
			api,
			instrument,
			price,
			balance,
			config.Fixture().Trading,
		)
		desk.fillsBySymbol = map[string][]Fill{
			"BTC/USD": {{
				ExecID: "fill-1",
				Side:   "buy",
				Qty:    decimal.NewFromFloat64(2),
				Price:  decimal.NewFromFloat64(10),
				Fee:    decimal.NewFromFloat64(0.1),
			}},
		}
		desk.historyReady = true

		Convey("When a balance snapshot rebuilds inventory from paper state", func() {
			desk.onBalances([]byte(`{
				"channel":"balances",
				"type":"snapshot",
				"sequence":1,
				"data":[
					{"asset":"USD","balance":100},
					{"asset":"BTC","balance":2}
				]
			}`))

			Convey("Then Desk owns the open position and enriched holding", func() {
				position, ok := desk.Position("BTC/USD")
				holding, err := balance.Holding("BTC/USD")

				So(ok, ShouldBeTrue)
				So(position.Status(), ShouldEqual, types.OPEN)
				So(err, ShouldBeNil)
				So(holding.Status, ShouldEqual, types.OPEN)
				So(holding.EntryPrice.Float64(), ShouldEqual, 10.0)
				So(holding.EntryFee.Float64(), ShouldEqual, 0.1)
				So(holding.Stoploss, ShouldNotBeNil)
				So(desk.OpenPositions(), ShouldEqual, 1)
				So(private.Writes(), ShouldHaveLength, 0)
			})

			Convey("And ticker updates immediately repaint live economics", func() {
				desk.onTicker([]byte(`{
					"channel":"ticker",
					"type":"update",
					"data":[{
						"symbol":"BTC/USD",
						"last":"12.1",
						"bid":"12.0",
						"ask":"12.2"
					}]
				}`))

				holding, err := balance.Holding("BTC/USD")

				So(err, ShouldBeNil)
				So(holding.Mark.Float64(), ShouldEqual, 12.0)
				So(holding.PnL, ShouldNotBeNil)
				So(holding.PnL.Float64(), ShouldEqual, 3.876)
				So(holding.ReturnPct, ShouldNotBeNil)
				So(*holding.ReturnPct, ShouldBeGreaterThan, 0)
			})
		})
	})
}

/*
BenchmarkDeskBalanceAdopt measures the restart balance path that converts
wallet-backed lots into Desk positions and derives entry economics from fills.
*/
func BenchmarkDeskBalanceAdopt(b *testing.B) {
	for b.Loop() {
		public := mockapi.NewConn(b.Context())
		private := mockapi.NewConn(b.Context())
		api := websocket.NewAPI(b.Context(), public, private, nil)

		if err := api.Initialize(); err != nil {
			b.Fatal(err)
		}

		price := NewPrice(api)

		if err := price.RememberFee("BTC/USD", kraken.TradeVolumeFee{
			Fee: decimal.NewFromFloat64(0.1),
		}); err != nil {
			b.Fatal(err)
		}

		instrument := NewInstrument(api, price, make(chan []byte, 4), config.Fixture().Market)
		instrument.Remember(kraken.InstrumentPair{
			Symbol:        "BTC/USD",
			Base:          "BTC",
			Quote:         "USD",
			Status:        "online",
			CostPrecision: 4,
			QtyIncrement:  decimal.NewFromFloat64(0.00000001),
		})
		balance := NewBalance(api, nil, make(chan []byte, 4), config.Fixture().Market)
		desk := NewDesk(
			context.Background(), api, instrument, price, balance,
			config.Fixture().Trading,
		)
		desk.fillsBySymbol = map[string][]Fill{
			"BTC/USD": {{
				ExecID: "fill-1",
				Side:   "buy",
				Qty:    decimal.NewFromFloat64(2),
				Price:  decimal.NewFromFloat64(10),
				Fee:    decimal.NewFromFloat64(0.1),
			}},
		}
		desk.historyReady = true
		desk.onBalances([]byte(`{"channel":"balances","type":"snapshot","sequence":1,"data":[{"asset":"USD","balance":100},{"asset":"BTC","balance":2}]}`))
		desk.onTicker([]byte(`{"channel":"ticker","type":"update","data":[{"symbol":"BTC/USD","last":"12.1","bid":"12.0","ask":"12.2"}]}`))
	}
}
