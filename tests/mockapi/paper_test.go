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
awaitBytes waits until n []byte frames arrive on an Actor subscription.
*/
func awaitBytes(channel chan any, n int) [][]byte {
	frames := make([][]byte, 0, n)
	deadline := time.Now().Add(2 * time.Second)

	for len(frames) < n && time.Now().Before(deadline) {
		select {
		case frame := <-channel:
			frames = append(frames, frame.([]byte))
		case <-time.After(5 * time.Millisecond):
		}
	}

	return frames
}

/*
takeBytes drains ready []byte frames without waiting.
*/
func takeBytes(channel chan any) [][]byte {
	frames := make([][]byte, 0)

	for {
		select {
		case frame := <-channel:
			frames = append(frames, frame.([]byte))
		default:
			return frames
		}
	}
}

/*
TestPaperHandle proves market and resting limit orders close the deterministic
acknowledgement, execution, balance, and open-order loop on Actor roots.
*/
func TestPaperHandle(t *testing.T) {
	Convey("Given a paper venue at a controlled touch", t, func() {
		bid := 99.0
		ask := 101.0
		conn := NewConn(t.Context(), "SIM1/USD")
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

		ackSub := conn.Subscribe("add_order")
		execSub := conn.Subscribe("executions")
		balSub := conn.Subscribe("balances")

		So(conn.Write(json.RawMessage(
			`{"method":"subscribe","params":{"channel":"executions"}}`,
		)), ShouldBeNil)
		So(conn.Write(json.RawMessage(
			`{"method":"subscribe","params":{"channel":"balances"}}`,
		)), ShouldBeNil)
		takeBytes(balSub.Channel)
		takeBytes(execSub.Channel)

		Convey("A market buy fills at the ask and updates both wallets", func() {
			request := json.RawMessage(`{"method":"add_order","req_id":7,"params":{` +
				`"order_type":"market","side":"buy","order_qty":2,"symbol":"SIM1/USD"}}`)
			So(conn.Write(request), ShouldBeNil)
			So(len(ackSub.Channel), ShouldEqual, 0)
			So(len(execSub.Channel), ShouldEqual, 0)
			So(conn.Drain(), ShouldBeNil)
			acks := awaitBytes(ackSub.Channel, 1)
			executions := awaitBytes(execSub.Channel, 1)
			balances := awaitBytes(balSub.Channel, 1)
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
			executions := awaitBytes(execSub.Channel, 1)
			So(executions, ShouldHaveLength, 1)
			So(string(executions[0]), ShouldContainSubstring, `"order_status":"open"`)
			So(conn.Write(json.RawMessage(
				`{"method":"subscribe","params":{"channel":"executions"}}`,
			)), ShouldBeNil)
			takeBytes(execSub.Channel)
			ask = 100.0
			So(conn.MatchPaper(), ShouldBeNil)
			So(conn.Drain(), ShouldBeNil)
			executions = awaitBytes(execSub.Channel, 1)
			balances := awaitBytes(balSub.Channel, 1)
			So(executions, ShouldHaveLength, 1)
			So(balances, ShouldHaveLength, 1)
			So(string(executions[0]), ShouldContainSubstring, `"order_status":"filled"`)
			open, err = conn.OpenOrders()
			So(err, ShouldBeNil)
			So(open, ShouldBeEmpty)
		})

		Convey("A fresh balances subscription republishes the current wallets", func() {
			So(conn.Write(json.RawMessage(
				`{"method":"subscribe","params":{"channel":"balances"}}`,
			)), ShouldBeNil)
			current := awaitBytes(balSub.Channel, 1)
			So(current, ShouldHaveLength, 1)

			var snapshot struct {
				Data []struct {
					Asset     string
					Balance   json.Number
					Available json.Number
				}
			}

			So(json.Unmarshal(current[0], &snapshot), ShouldBeNil)
			So(len(snapshot.Data), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given paper handlers without accepted private subscriptions", t, func() {
		conn := NewConn(t.Context(), "SIM1/USD")
		So(conn.EnablePaper(PaperOptions{
			Quote: func(string) (float64, float64, float64, float64, bool) {
				return 99, 10, 101, 10, true
			},
			Now:      time.Now,
			Balances: map[string]float64{"USD": 1000},
			MakerFee: 0.005,
			TakerFee: 0.01,
		}), ShouldBeNil)
		ackSub := conn.Subscribe("add_order")
		execSub := conn.Subscribe("executions")
		balSub := conn.Subscribe("balances")
		request := json.RawMessage(`{"method":"add_order","req_id":1,"params":{` +
			`"order_type":"market","side":"buy","order_qty":1,"symbol":"SIM1/USD"}}`)
		So(conn.Write(request), ShouldBeNil)
		So(conn.Drain(), ShouldBeNil)
		So(awaitBytes(ackSub.Channel, 1), ShouldHaveLength, 1)
		time.Sleep(20 * time.Millisecond)
		So(takeBytes(execSub.Channel), ShouldBeEmpty)
		So(takeBytes(balSub.Channel), ShouldBeEmpty)
	})
}

/*
TestPaperMatch proves a batch that exceeds shared touch depth leaves balances,
reservations, open orders, and private sequence counters unchanged.
*/
func TestPaperMatch(t *testing.T) {
	Convey("Given two resting orders whose total exceeds displayed liquidity", t, func() {
		ask := 101.0
		conn := NewConn(t.Context(), "SIM1/USD")
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
	conn := NewConn(b.Context(), "SIM1/USD")
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

	if err := conn.Write(json.RawMessage(
		`{"method":"subscribe","params":{"channel":"executions"}}`,
	)); err != nil {
		b.Fatal(err)
	}

	if err := conn.Write(json.RawMessage(
		`{"method":"subscribe","params":{"channel":"balances"}}`,
	)); err != nil {
		b.Fatal(err)
	}

	request := json.RawMessage(fmt.Sprintf(
		`{"method":"add_order","params":{"order_type":"market","side":"buy",`+
			`"order_qty":1,"symbol":"SIM1/USD"}}`,
	))
	b.ReportAllocs()

	for b.Loop() {
		if err := conn.Write(request); err != nil {
			b.Fatal(err)
		}

		if err := conn.Drain(); err != nil {
			b.Fatal(err)
		}
	}
}
