package websocket

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken"

	. "github.com/smartystreets/goconvey/convey"
)

type jsonPayload []byte

func (payload jsonPayload) MarshalJSON() ([]byte, error) {
	return payload, nil
}

type frameCapture struct {
	mu       sync.Mutex
	payloads [][]byte
}

func (capture *frameCapture) record(raw []byte) {
	capture.mu.Lock()
	capture.payloads = append(capture.payloads, append([]byte(nil), raw...))
	capture.mu.Unlock()
}

func (capture *frameCapture) count() int {
	capture.mu.Lock()
	defer capture.mu.Unlock()

	return len(capture.payloads)
}

func (capture *frameCapture) last() []byte {
	capture.mu.Lock()
	defer capture.mu.Unlock()

	if len(capture.payloads) == 0 {
		return nil
	}

	return capture.payloads[len(capture.payloads)-1]
}

func waitCaptureCount(
	t *testing.T,
	capture *frameCapture,
	expected int,
) {
	deadline := time.Now().Add(time.Second)

	for time.Now().Before(deadline) {
		if capture.count() >= expected {
			return
		}

		time.Sleep(time.Millisecond)
	}

	t.Fatalf("expected %d frames, got %d", expected, capture.count())
}

func paperFixture(t *testing.T) *Paper {
	dir := t.TempDir()
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
"paper orders -o json")
	if [ -f %[1]q ]; then
		printf '%%s' '{"mode":"paper","open_orders":[{"id":"PAPER-00002","pair":"BTCUSD","price":110000,"reserved_amount":11,"reserved_asset":"USD","side":"buy","type":"limit","volume":0.0001,"created_at":"2026-07-05T10:01:00Z"}]}'
	else
		printf '%%s' '{"mode":"paper","open_orders":[{"id":"PAPER-00001","pair":"BTCUSD","price":100000,"reserved_amount":10,"reserved_asset":"USD","side":"buy","type":"limit","volume":0.0001,"created_at":"2026-07-05T10:00:00Z"}]}'
	fi
	;;
"paper buy -o json BTCUSD 0.0001")
	touch %[1]q
	printf '%%s' '{"id":"PAPER-00002","status":"accepted"}'
	;;
*)
	echo "unexpected: $*" >&2
	exit 2
	;;
esac
`, state)

	if err := os.WriteFile(command, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	pool := qpool.NewQ[any](t.Context(), 1, 1, nil)
	paper := NewPaper(t.Context(), pool, "", true)
	paper.cli = &kraken.PaperCLI{Command: command}

	return paper
}

func TestPaperOn(t *testing.T) {
	Convey("Given a paper transport backed by the Kraken CLI", t, func() {
		viper.Set("market.quote_currency", "USD")

		paper := paperFixture(t)

		balances := frameCapture{}
		executions := frameCapture{}
		orders := frameCapture{}

		paper.On("balances", balances.record)
		paper.On("executions", executions.record)
		paper.On("orders", orders.record)

		Convey("When channels are registered", func() {
			waitCaptureCount(t, &balances, 1)
			waitCaptureCount(t, &executions, 1)
			waitCaptureCount(t, &orders, 1)

			Convey("Then balances should publish immediately", func() {
				frame := kraken.Balance{}
				So(sonic.Unmarshal(balances.last(), &frame), ShouldBeNil)
				So(frame.Sequence, ShouldEqual, 1)
				rows := kraken.NewBalanceDataSlice(balances.last())
				So(*rows, ShouldHaveLength, 1)
				So((*rows)[0].Asset, ShouldEqual, "USD")
				So((*rows)[0].Balance.String(), ShouldEqual, "200")
			})

			Convey("Then executions should publish immediately", func() {
				rows := kraken.NewExecutionDataSlice(executions.last())
				So(*rows, ShouldHaveLength, 2)
				So((*rows)[0].ExecType, ShouldEqual, "snapshot")
				So((*rows)[0].PositionStatus, ShouldEqual, "open")
				So((*rows)[1].ExecID, ShouldEqual, "PAPER-00001")
			})

			Convey("Then orders should publish immediately", func() {
				rows := kraken.NewOrderDataSlice(orders.last())
				So(*rows, ShouldHaveLength, 1)
				So((*rows)[0].ID, ShouldEqual, "PAPER-00001")
				So((*rows)[0].Pair, ShouldEqual, "BTC/USD")
				So((*rows)[0].ReservedAmount.String(), ShouldEqual, "10")
			})
		})
	})
}

func TestPaperWrite(t *testing.T) {
	Convey("Given a paper transport with registered callbacks", t, func() {
		viper.Set("market.quote_currency", "USD")

		paper := paperFixture(t)

		orderResponses := frameCapture{}
		orders := frameCapture{}
		executions := frameCapture{}

		paper.On("add_order", orderResponses.record)
		paper.On("orders", orders.record)
		paper.On("executions", executions.record)

		order := &kraken.Order{
			Method: "add_order",
			Params: kraken.LimitOrderParams{
				OrderType: "market",
				Side:      "buy",
				OrderQty:  0.0001,
				Symbol:    "BTC/USD",
			},
		}

		payload, marshalErr := sonic.Marshal(order)
		So(marshalErr, ShouldBeNil)

		Convey("When a paper order is submitted", func() {
			err := paper.Write(jsonPayload(payload))

			Convey("Then the order response should publish", func() {
				So(err, ShouldBeNil)

				waitCaptureCount(t, &orderResponses, 1)

				response := kraken.NewOrderResponse(orderResponses.last())
				So(response.Success, ShouldBeTrue)
				So(response.Method, ShouldEqual, "add_order")
				So(response.Result.OrderID, ShouldEqual, "PAPER-00002")
			})

			Convey("Then orders should refresh", func() {
				waitCaptureCount(t, &orders, 2)

				rows := kraken.NewOrderDataSlice(orders.last())
				So(*rows, ShouldHaveLength, 1)
				So((*rows)[0].ID, ShouldEqual, "PAPER-00002")
				So((*rows)[0].Pair, ShouldEqual, "BTC/USD")
				So((*rows)[0].ReservedAmount.String(), ShouldEqual, "11")
			})

			Convey("Then executions should refresh", func() {
				waitCaptureCount(t, &executions, 2)

				rows := kraken.NewExecutionDataSlice(executions.last())
				frame := kraken.Execution{}
				So(sonic.Unmarshal(executions.last(), &frame), ShouldBeNil)
				So(frame.Sequence, ShouldEqual, 2)
				So(*rows, ShouldHaveLength, 3)
				So((*rows)[0].ExecType, ShouldEqual, "snapshot")
				So((*rows)[2].ExecID, ShouldEqual, "PAPER-00002")
			})
		})
	})
}
