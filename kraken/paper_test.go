package kraken

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"

	. "github.com/smartystreets/goconvey/convey"
)

func paperCLIFixture(t *testing.T) *PaperCLI {
	command := filepath.Join(t.TempDir(), "kraken")
	script := `#!/bin/sh
case "$*" in
"paper balance -o json")
	printf '%s' '{"balances":{"USD":{"available":125,"reserved":75,"total":200},"BTC":{"available":0.01,"reserved":0,"total":0.01}},"mode":"paper"}'
	;;
"paper history -o json")
	printf '%s' '{"trades":[{"cost":10,"fee":0.026,"id":"PAPER-00002","order_id":"PAPER-00001","pair":"BTCUSD","price":100000,"side":"buy","status":"filled","time":"2026-07-05T10:00:00Z","volume":0.0001}],"mode":"paper"}'
	;;
"paper orders -o json")
	printf '%s' '{"mode":"paper","open_orders":[{"id":"PAPER-00003","pair":"BTCUSD","price":90000,"reserved_amount":9,"reserved_asset":"USD","side":"buy","type":"limit","volume":0.0001,"created_at":"2026-07-05T10:02:00Z"}]}'
	;;
"paper buy -o json BTCUSD 0.0001")
	printf '%s' '{"id":"PAPER-00003"}'
	;;
"paper sell -o json --type limit --price 120000 BTCUSD 0.0001")
	printf '%s' '{"id":"PAPER-00004"}'
	;;
"paper cancel -o json PAPER-00004")
	printf '%s' '{"id":"PAPER-00004","status":"cancelled"}'
	;;
*)
	echo "unexpected: $*" >&2
	exit 2
	;;
esac
`

	if err := os.WriteFile(command, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	return &PaperCLI{Command: command}
}

func TestBalances(t *testing.T) {
	Convey("Given the Kraken paper CLI adapter", t, func() {
		viper.Set("market.quote_currency", "USD")
		paper := paperCLIFixture(t)

		Convey("When balances are read", func() {
			frame, err := paper.Balances(context.Background())

			Convey("Then they should be a balances channel snapshot", func() {
				So(err, ShouldBeNil)
				So(frame.Channel, ShouldEqual, "balances")
				So(frame.Type, ShouldEqual, "snapshot")
				So(frame.Data, ShouldHaveLength, 2)
				So(frame.Data[0].Asset, ShouldEqual, "BTC")
				So(frame.Data[0].AssetClass, ShouldEqual, "currency")
				So(frame.Data[1].Asset, ShouldEqual, "USD")
				So(frame.Data[1].Balance.String(), ShouldEqual, "200")
				So(frame.Data[1].Available.String(), ShouldEqual, "125")
				So(frame.Data[1].Reserved.String(), ShouldEqual, "75")
			})
		})
	})
}

func TestExecutions(t *testing.T) {
	Convey("Given the Kraken paper CLI adapter", t, func() {
		viper.Set("market.quote_currency", "USD")
		paper := paperCLIFixture(t)

		Convey("When executions are read", func() {
			frame, err := paper.Executions(context.Background())

			Convey("Then they should be an executions channel snapshot", func() {
				So(err, ShouldBeNil)
				So(frame.Channel, ShouldEqual, "executions")
				So(frame.Type, ShouldEqual, "snapshot")
				So(frame.Data, ShouldHaveLength, 2)
				So(frame.Data[0].ExecType, ShouldEqual, "snapshot")
				So(frame.Data[0].PositionStatus, ShouldEqual, "open")
				So(frame.Data[0].Symbol, ShouldEqual, "BTC/USD")
				So(frame.Data[0].LastQty, ShouldEqual, 0.0001)
				So(frame.Data[0].Side, ShouldEqual, "buy")
				So(frame.Data[1].ExecID, ShouldEqual, "PAPER-00002")
				So(frame.Data[1].OrderID, ShouldEqual, "PAPER-00001")
				So(frame.Data[1].Symbol, ShouldEqual, "BTC/USD")
				So(frame.Data[1].FeeUSDEquiv.String(), ShouldEqual, "0.026")
				So(frame.Data[1].OrderQty, ShouldEqual, 0.0001)
				So(frame.Data[1].OrderStatus, ShouldEqual, "filled")
			})
		})
	})
}

func TestOrders(t *testing.T) {
	Convey("Given the Kraken paper CLI adapter", t, func() {
		viper.Set("market.quote_currency", "USD")
		paper := paperCLIFixture(t)

		Convey("When orders are read", func() {
			frame, err := paper.Orders(context.Background())

			Convey("Then they should be an orders channel snapshot", func() {
				So(err, ShouldBeNil)
				So(frame.Channel, ShouldEqual, "orders")
				So(frame.Type, ShouldEqual, "snapshot")
				So(frame.Data, ShouldHaveLength, 1)
				So(frame.Data[0].ID, ShouldEqual, "PAPER-00003")
				So(frame.Data[0].Pair, ShouldEqual, "BTC/USD")
				So(frame.Data[0].ReservedAmount.String(), ShouldEqual, "9")
				So(frame.Data[0].ReservedAsset, ShouldEqual, "USD")
				So(frame.Data[0].Type, ShouldEqual, "limit")
			})
		})
	})
}

func TestSubmit(t *testing.T) {
	Convey("Given the Kraken paper CLI adapter", t, func() {
		viper.Set("market.quote_currency", "USD")
		paper := paperCLIFixture(t)

		Convey("When a buy order is submitted", func() {
			response, err := paper.Submit(context.Background(), &Order{
				Method: "add_order",
				Params: LimitOrderParams{
					OrderType: "market",
					Side:      "buy",
					OrderQty:  0.0001,
					Symbol:    "BTC/USD",
				},
			})

			Convey("Then it should call the paper buy command", func() {
				So(err, ShouldBeNil)
				So(response.Success, ShouldBeTrue)
				So(response.Method, ShouldEqual, "add_order")
				So(response.Result.OrderID, ShouldEqual, "PAPER-00003")
			})
		})

		Convey("When a limit sell order is submitted", func() {
			response, err := paper.Submit(context.Background(), &Order{
				Method: "add_order",
				Params: LimitOrderParams{
					OrderType:  "limit",
					Side:       "sell",
					LimitPrice: 120000,
					OrderQty:   0.0001,
					Symbol:     "BTC/USD",
				},
			})

			Convey("Then it should call the paper sell command with limit options", func() {
				So(err, ShouldBeNil)
				So(response.Success, ShouldBeTrue)
				So(response.Result.OrderID, ShouldEqual, "PAPER-00004")
			})
		})

		Convey("When a cancel order is submitted", func() {
			response, err := paper.Submit(context.Background(), &Order{
				Method: "cancel_order",
				Params: map[string]any{
					"order_id": "PAPER-00004",
				},
			})

			Convey("Then it should call the paper cancel command", func() {
				So(err, ShouldBeNil)
				So(response.Success, ShouldBeTrue)
				So(response.Result.OrderID, ShouldEqual, "PAPER-00004")
			})
		})
	})
}

func TestPost(t *testing.T) {
	Convey("Given the Kraken paper CLI adapter", t, func() {
		previousTaker := viper.GetFloat64("trading.paper.taker_fee_bps")
		previousMaker := viper.GetFloat64("trading.paper.maker_fee_bps")
		viper.Set("trading.paper.taker_fee_bps", 26)
		viper.Set("trading.paper.maker_fee_bps", 16)
		defer func() {
			viper.Set("trading.paper.taker_fee_bps", previousTaker)
			viper.Set("trading.paper.maker_fee_bps", previousMaker)
		}()

		paper := &PaperCLI{Command: "kraken"}

		Convey("When TradeVolume is posted", func() {
			body, err := paper.Post(
				context.Background(),
				TradeVolumeEndpoint,
				NewTradeVolumeRequest([]string{"BTC/USD"}),
			)

			Convey("Then it should return configured fee rates", func() {
				So(err, ShouldBeNil)

				schedule := FeeSchedule{}
				So(sonic.Unmarshal(body, &schedule), ShouldBeNil)
				So(schedule.Pairs["BTC/USD"].Taker, ShouldAlmostEqual, 0.0026, 1e-12)
				So(schedule.Pairs["BTC/USD"].Maker, ShouldAlmostEqual, 0.0016, 1e-12)
			})
		})
	})
}

func BenchmarkExecutions(b *testing.B) {
	viper.Set("market.quote_currency", "USD")

	command := filepath.Join(b.TempDir(), "kraken")
	script := `#!/bin/sh
case "$*" in
"paper history -o json")
	printf '%s' '{"trades":[{"cost":10,"fee":0.026,"id":"PAPER-00002","order_id":"PAPER-00001","pair":"BTCUSD","price":100000,"side":"buy","status":"filled","time":"2026-07-05T10:00:00Z","volume":0.0001}],"mode":"paper"}'
	;;
*)
	echo "unexpected: $*" >&2
	exit 2
	;;
esac
`

	if err := os.WriteFile(command, []byte(script), 0o755); err != nil {
		b.Fatal(err)
	}

	paper := &PaperCLI{Command: command}

	b.ReportAllocs()
	for b.Loop() {
		frame, err := paper.Executions(context.Background())

		if err != nil {
			b.Fatal(err)
		}

		if len(frame.Data) != 2 {
			b.Fatal("expected snapshot plus one paper execution")
		}
	}
}
