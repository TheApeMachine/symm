package kraken

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPaperCLI(testingTB *testing.T) {
	Convey("Given the Kraken paper CLI adapter", testingTB, func() {
		command := filepath.Join(testingTB.TempDir(), "kraken")
		script := `#!/bin/sh
case "$*" in
"paper balance -o json")
	printf '%s' '{"balances":{"USD":{"available":125,"reserved":75,"total":200},"BTC":{"available":0.01,"reserved":0,"total":0.01}},"mode":"paper"}'
	;;
"paper history -o json")
	printf '%s' '{"trades":[{"cost":10,"fee":0.026,"id":"PAPER-00002","order_id":"PAPER-00001","pair":"BTCUSD","price":100000,"side":"buy","status":"filled","time":"2026-07-05T10:00:00Z","volume":0.0001}],"mode":"paper"}'
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

		So(os.WriteFile(command, []byte(script), 0o755), ShouldBeNil)

		paper := &PaperCLI{Command: command}

		Convey("When balances are read", func() {
			rows, err := paper.Balances(context.Background())

			Convey("Then they should be adapted into Kraken balance rows", func() {
				So(err, ShouldBeNil)
				So(rows, ShouldHaveLength, 2)
				So(rows[0].Asset, ShouldEqual, "BTC")
				So(rows[1].Asset, ShouldEqual, "USD")
				So(rows[1].Balance, ShouldEqual, 200)
				So(rows[1].Available, ShouldEqual, 125)
				So(rows[1].Reserved, ShouldEqual, 75)
			})
		})

		Convey("When history is read", func() {
			rows, err := paper.Executions(context.Background())

			Convey("Then it should be adapted into Kraken execution rows", func() {
				So(err, ShouldBeNil)
				So(rows, ShouldHaveLength, 1)
				So(rows[0].ExecID, ShouldEqual, "PAPER-00002")
				So(rows[0].OrderID, ShouldEqual, "PAPER-00001")
				So(rows[0].Symbol, ShouldEqual, "BTCUSD")
				So(rows[0].OrderQty, ShouldEqual, 0.0001)
				So(rows[0].OrderStatus, ShouldEqual, "filled")
			})
		})

		Convey("When a buy order is submitted", func() {
			err := paper.Submit(context.Background(), &Order{
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
			})
		})

		Convey("When a limit sell order is submitted", func() {
			err := paper.Submit(context.Background(), &Order{
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
			})
		})

		Convey("When a cancel order is submitted", func() {
			err := paper.Submit(context.Background(), &Order{
				Method: "cancel_order",
				Params: map[string]any{
					"order_id": "PAPER-00004",
				},
			})

			Convey("Then it should call the paper cancel command", func() {
				So(err, ShouldBeNil)
			})
		})
	})
}
