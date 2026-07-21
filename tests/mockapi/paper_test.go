package mockapi

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestPaperHandle proves market and resting limit orders close the deterministic
acknowledgement, execution, balance, and open-order loop.
*/
func TestPaperHandle(t *testing.T) {
	Convey("Given a paper venue at a controlled touch", t, func() {
		bid := 99.0
		ask := 101.0
		conn := NewConn("SIM1/USD")
		So(conn.EnablePaper(PaperOptions{
			Quote: func(string) (float64, float64, bool) {
				return bid, ask, true
			},
			Now: func() time.Time {
				return time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)
			},
			Balances: map[string]float64{"USD": 1000},
			FeeRate:  0.01,
		}), ShouldBeNil)
		acks := [][]byte{}
		executions := [][]byte{}
		balances := [][]byte{}
		conn.On("add_order", func(payload []byte) { acks = append(acks, payload) })
		conn.On("executions", func(payload []byte) { executions = append(executions, payload) })
		conn.On("balances", func(payload []byte) { balances = append(balances, payload) })

		Convey("A market buy fills at the ask and updates both wallets", func() {
			request := json.RawMessage(`{"method":"add_order","req_id":7,"params":{` +
				`"order_type":"market","side":"buy","order_qty":2,"symbol":"SIM1/USD"}}`)
			So(conn.Write(request), ShouldBeNil)
			So(acks, ShouldBeEmpty)
			So(executions, ShouldBeEmpty)
			So(conn.Drain(), ShouldBeNil)
			So(acks, ShouldHaveLength, 1)
			So(executions, ShouldHaveLength, 1)
			So(balances, ShouldHaveLength, 1)
			So(string(executions[0]), ShouldContainSubstring, `"last_price":"101.00000000"`)
			So(string(balances[0]), ShouldContainSubstring, `"asset":"SIM1"`)
			So(string(balances[0]), ShouldContainSubstring, `"balance":2`)
			So(string(balances[0]), ShouldContainSubstring, `"balance":795.98`)
		})

		Convey("A non-crossing limit rests until the market reaches it", func() {
			request := json.RawMessage(`{"method":"add_order","req_id":8,"params":{` +
				`"order_type":"limit","side":"buy","limit_price":100,` +
				`"order_qty":1,"symbol":"SIM1/USD"}}`)
			So(conn.Write(request), ShouldBeNil)
			So(conn.Drain(), ShouldBeNil)
			open, err := conn.OpenOrders()
			So(err, ShouldBeNil)
			So(open, ShouldHaveLength, 1)
			So(executions, ShouldHaveLength, 1)
			So(string(executions[0]), ShouldContainSubstring, `"order_status":"open"`)

			ask = 100
			So(conn.MatchPaper(), ShouldBeNil)
			So(conn.Drain(), ShouldBeNil)
			open, err = conn.OpenOrders()
			So(err, ShouldBeNil)
			So(open, ShouldBeEmpty)
			So(executions, ShouldHaveLength, 2)
			So(string(executions[1]), ShouldContainSubstring, `"order_status":"filled"`)
		})
	})
}
