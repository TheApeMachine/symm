package websocket

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/theapemachine/symm/kraken"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPaperPrivate(testingTB *testing.T) {
	Convey("Given a paper private stream backed by the Kraken CLI", testingTB, func() {
		dir := testingTB.TempDir()
		command := filepath.Join(dir, "kraken")
		state := filepath.Join(dir, "submitted")
		script := fmt.Sprintf(`#!/bin/sh
case "$*" in
"paper balance -o json")
	printf '%%s' '{"balances":{"USD":{"available":198,"reserved":2,"total":200}},"mode":"paper"}'
	;;
"paper history -o json")
	if [ -f %[1]q ]; then
		printf '%%s' '{"trades":[{"cost":10,"fee":0.026,"id":"PAPER-00001","order_id":"PAPER-00001","pair":"BTCUSD","price":100000,"side":"buy","status":"filled","time":"2026-07-05T10:00:00Z","volume":0.0001},{"cost":11,"fee":0.0286,"id":"PAPER-00002","order_id":"PAPER-00002","pair":"BTCUSD","price":110000,"side":"buy","status":"filled","time":"2026-07-05T10:01:00Z","volume":0.0001}],"mode":"paper"}'
	else
		printf '%%s' '{"trades":[{"cost":10,"fee":0.026,"id":"PAPER-00001","order_id":"PAPER-00001","pair":"BTCUSD","price":100000,"side":"buy","status":"filled","time":"2026-07-05T10:00:00Z","volume":0.0001}],"mode":"paper"}'
	fi
	;;
"paper buy -o json BTCUSD 0.0001")
	touch %[1]q
	printf '%%s' '{"id":"PAPER-00002"}'
	;;
*)
	echo "unexpected: $*" >&2
	exit 2
	;;
esac
`, state)

		So(os.WriteFile(command, []byte(script), 0o755), ShouldBeNil)

		private := NewPaperPrivate(context.Background())
		private.paper = &kraken.PaperCLI{Command: command}
		balances := private.Observe("balances")
		executions := private.Observe("executions")

		Convey("When observers subscribe", func() {
			Convey("Then balances should publish immediately and old executions should not replay", func() {
				select {
				case observed := <-balances:
					rows := kraken.NewBalanceDataSlice(observed)
					So(*rows, ShouldHaveLength, 1)
					So((*rows)[0].Asset, ShouldEqual, "USD")
					So((*rows)[0].Balance, ShouldEqual, 200)
				case <-time.After(time.Second):
					testingTB.Fatal("paper balances were not published")
				}

				select {
				case observed := <-executions:
					testingTB.Fatalf("old paper execution replayed: %s", observed)
				default:
				}
			})
		})

		Convey("When a paper order is submitted", func() {
			select {
			case <-balances:
			case <-time.After(time.Second):
				testingTB.Fatal("initial paper balance was not published")
			}

			err := private.Submit(&kraken.Order{
				Method: "add_order",
				Params: kraken.LimitOrderParams{
					OrderType: "market",
					Side:      "buy",
					OrderQty:  0.0001,
					Symbol:    "BTC/USD",
				},
			})

			Convey("Then the new execution should publish through the private stream", func() {
				So(err, ShouldBeNil)

				select {
				case observed := <-executions:
					rows := kraken.NewExecutionDataSlice(observed)
					So(*rows, ShouldHaveLength, 1)
					So((*rows)[0].ExecID, ShouldEqual, "PAPER-00002")
				case <-time.After(time.Second):
					testingTB.Fatal("paper execution was not published")
				}
			})
		})
	})
}
