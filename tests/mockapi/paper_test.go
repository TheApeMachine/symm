package mockapi

import (
	"encoding/json"
	"fmt"
	"maps"
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
			Quote: func(string) (float64, float64, float64, float64, bool) {
				return bid, 10, ask, 10, true
			},
			Now: func() time.Time {
				return time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)
			},
			Balances: map[string]float64{"USD": 1000},
			MakerFee: 0.005,
			TakerFee: 0.01,
		}), ShouldBeNil)
		acks := [][]byte{}
		executions := [][]byte{}
		balances := [][]byte{}
		conn.On("add_order", func(payload []byte) { acks = append(acks, payload) })
		conn.On("executions", func(payload []byte) { executions = append(executions, payload) })
		conn.On("balances", func(payload []byte) { balances = append(balances, payload) })
		So(conn.Write(json.RawMessage(
			`{"method":"subscribe","params":{"channel":"executions"}}`,
		)), ShouldBeNil)
		So(conn.Write(json.RawMessage(
			`{"method":"subscribe","params":{"channel":"balances"}}`,
		)), ShouldBeNil)
		executions = nil
		balances = nil

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
			So(open["PAPER-00001"].Description.Pair, ShouldEqual, "SIM1/USD")
			So(open["PAPER-00001"].Description.Type, ShouldEqual, "buy")
			So(open["PAPER-00001"].Volume.Float64(), ShouldEqual, 1.0)
			So(executions, ShouldHaveLength, 1)
			So(string(executions[0]), ShouldContainSubstring, `"order_status":"open"`)
			So(conn.Write(json.RawMessage(
				`{"method":"subscribe","params":{"channel":"executions"}}`,
			)), ShouldBeNil)
			So(executions, ShouldHaveLength, 2)
			So(string(executions[1]), ShouldContainSubstring, `"type":"snapshot"`)
			So(string(executions[1]), ShouldContainSubstring, `"order_id":"PAPER-00001"`)

			ask = 100
			So(conn.MatchPaper(), ShouldBeNil)
			So(conn.Drain(), ShouldBeNil)
			open, err = conn.OpenOrders()
			So(err, ShouldBeNil)
			So(open, ShouldBeEmpty)
			So(executions, ShouldHaveLength, 3)
			So(string(executions[2]), ShouldContainSubstring, `"order_status":"filled"`)
		})

		Convey("Resting limits reserve funds and prevent collective overcommitment", func() {
			first := json.RawMessage(`{"method":"add_order","req_id":9,"params":{` +
				`"order_type":"limit","side":"buy","limit_price":100,` +
				`"order_qty":1,"symbol":"SIM1/USD"}}`)
			second := json.RawMessage(`{"method":"add_order","req_id":10,"params":{` +
				`"order_type":"limit","side":"buy","limit_price":100,` +
				`"order_qty":9,"symbol":"SIM1/USD"}}`)
			So(conn.Write(first), ShouldBeNil)
			So(conn.Drain(), ShouldBeNil)
			So(conn.Write(second), ShouldNotBeNil)
			current := [][]byte{}
			conn.On("balances", func(payload []byte) { current = append(current, payload) })
			So(conn.Write(json.RawMessage(
				`{"method":"subscribe","params":{"channel":"balances"}}`,
			)), ShouldBeNil)
			So(current, ShouldHaveLength, 1)
			var snapshot struct {
				Data []struct {
					Asset     string  `json:"asset"`
					Available float64 `json:"available"`
					Reserved  float64 `json:"reserved"`
				} `json:"data"`
			}
			So(json.Unmarshal(current[0], &snapshot), ShouldBeNil)
			So(snapshot.Data, ShouldHaveLength, 1)
			So(snapshot.Data[0].Asset, ShouldEqual, "USD")
			So(snapshot.Data[0].Available, ShouldAlmostEqual, 899.5)
			So(snapshot.Data[0].Reserved, ShouldAlmostEqual, 100.5)
		})

		Convey("Multiple crossing limits fill in insertion order", func() {
			for _, requestID := range []int{11, 12} {
				request := json.RawMessage(fmt.Sprintf(
					`{"method":"add_order","req_id":%d,"params":{`+
						`"order_type":"limit","side":"buy","limit_price":100,`+
						`"order_qty":1,"symbol":"SIM1/USD"}}`,
					requestID,
				))
				So(conn.Write(request), ShouldBeNil)
				So(conn.Drain(), ShouldBeNil)
			}

			ask = 100
			So(conn.MatchPaper(), ShouldBeNil)
			So(conn.Drain(), ShouldBeNil)
			So(executions, ShouldHaveLength, 4)
			So(string(executions[2]), ShouldContainSubstring, `"order_id":"PAPER-00001"`)
			So(string(executions[3]), ShouldContainSubstring, `"order_id":"PAPER-00002"`)
		})

		Convey("A rejected request does not consume a venue order identity", func() {
			So(conn.Write(json.RawMessage(
				`{"method":"add_order","params":{"order_type":"limit",`+
					`"side":"buy","limit_price":100,"order_qty":1e999,`+
					`"symbol":"SIM1/USD"}}`,
			)), ShouldNotBeNil)
			So(conn.Write(json.RawMessage(
				`{"method":"add_order","params":{"order_type":"limit",`+
					`"side":"buy","limit_price":100,"order_qty":0,"symbol":"SIM1/USD"}}`,
			)), ShouldNotBeNil)
			So(conn.Write(json.RawMessage(
				`{"method":"add_order","params":{"order_type":"limit",`+
					`"side":"buy","limit_price":100,"order_qty":1,"symbol":"SIM1/USD"}}`,
			)), ShouldBeNil)
			open, err := conn.OpenOrders()
			So(err, ShouldBeNil)
			So(open, ShouldContainKey, "PAPER-00001")
		})
	})

	Convey("Given paper handlers without accepted private subscriptions", t, func() {
		conn := NewConn("SIM1/USD")
		So(conn.EnablePaper(PaperOptions{
			Quote: func(string) (float64, float64, float64, float64, bool) {
				return 99, 10, 101, 10, true
			},
			Now:      time.Now,
			Balances: map[string]float64{"USD": 1000},
			MakerFee: 0.005,
			TakerFee: 0.01,
		}), ShouldBeNil)
		acks := 0
		executions := 0
		balances := 0
		conn.On("add_order", func([]byte) { acks++ })
		conn.On("executions", func([]byte) { executions++ })
		conn.On("balances", func([]byte) { balances++ })

		Convey("An order response should not bypass the private venue boundary", func() {
			So(conn.Write(json.RawMessage(
				`{"method":"add_order","params":{"order_type":"market",`+
					`"side":"buy","order_qty":1,"symbol":"SIM1/USD"}}`,
			)), ShouldBeNil)
			So(conn.Drain(), ShouldBeNil)
			So(acks, ShouldEqual, 1)
			So(executions, ShouldEqual, 0)
			So(balances, ShouldEqual, 0)
		})
	})
}

/*
TestPaperMatch proves a batch that exceeds shared touch depth leaves balances,
reservations, open orders, and private sequence counters unchanged.
*/
func TestPaperMatch(t *testing.T) {
	Convey("Given two resting orders whose total exceeds displayed liquidity", t, func() {
		ask := 101.0
		conn := NewConn("SIM1/USD")
		So(conn.EnablePaper(PaperOptions{
			Quote: func(string) (float64, float64, float64, float64, bool) {
				return 99, 10, ask, 10, true
			},
			Now:      time.Now,
			Balances: map[string]float64{"USD": 10_000},
			MakerFee: 0.005,
			TakerFee: 0.01,
		}), ShouldBeNil)

		for requestID := range 2 {
			request := json.RawMessage(fmt.Sprintf(
				`{"method":"add_order","req_id":%d,"params":{`+
					`"order_type":"limit","side":"buy","limit_price":100,`+
					`"order_qty":6,"symbol":"SIM1/USD"}}`,
				requestID,
			))
			So(conn.Write(request), ShouldBeNil)
			So(conn.Drain(), ShouldBeNil)
		}

		beforeBalances := maps.Clone(conn.paper.balances)
		beforeReserved := maps.Clone(conn.paper.reserved)
		beforeExecutions := conn.paper.nextExec
		ask = 100

		Convey("Matching should reject and roll back the complete batch", func() {
			So(conn.MatchPaper(), ShouldNotBeNil)
			So(conn.paper.balances, ShouldResemble, beforeBalances)
			So(conn.paper.reserved, ShouldResemble, beforeReserved)
			So(conn.paper.nextExec, ShouldEqual, beforeExecutions)
			open, err := conn.OpenOrders()
			So(err, ShouldBeNil)
			So(open, ShouldHaveLength, 2)
		})
	})
}

/*
BenchmarkPaper_Handle measures validation, execution, and ledger rendering for
one touch-sized paper order through the mock connection boundary.
*/
func BenchmarkPaper_Handle(b *testing.B) {
	conn := NewConn("SIM1/USD")
	at := time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)

	if err := conn.EnablePaper(PaperOptions{
		Quote: func(string) (float64, float64, float64, float64, bool) {
			return 99, 10_000, 101, 10_000, true
		},
		Now:      func() time.Time { return at },
		Balances: map[string]float64{"USD": 1_000_000_000},
		MakerFee: 0.005,
		TakerFee: 0.01,
	}); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	request := json.RawMessage(`{"method":"add_order","params":{` +
		`"order_type":"market","side":"buy","order_qty":1,"symbol":"SIM1/USD"}}`)

	for b.Loop() {
		if err := conn.Write(request); err != nil {
			b.Fatal(err)
		}

		if err := conn.Drain(); err != nil {
			b.Fatal(err)
		}
	}
}
