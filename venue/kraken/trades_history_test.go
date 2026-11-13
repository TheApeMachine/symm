package kraken

import (
	"testing"
	"time"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
)

func TestNewTradesHistoryRequest(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewTradesHistoryRequest is marshaled with nil symbols", func() {
			raw, err := sonic.Marshal(NewTradesHistoryRequest(nil))
			So(err, ShouldBeNil)

			var payload map[string]any
			So(sonic.Unmarshal(raw, &payload), ShouldBeNil)

			Convey("only Kraken-valid fields are sent", func() {
				So(payload["type"], ShouldEqual, "all")
				So(payload["limit"], ShouldEqual, 100)
				So(payload["without_count"], ShouldEqual, true)
				So(payload["with_cursor"], ShouldEqual, true)
				_, hasPair := payload["pair"]
				So(hasPair, ShouldBeFalse)
				_, hasRebase := payload["rebase_multiplier"]
				So(hasRebase, ShouldBeFalse)
				_, hasAclass := payload["aclass"]
				So(hasAclass, ShouldBeFalse)
				_, hasCursor := payload["cursor"]
				So(hasCursor, ShouldBeFalse)
			})
		})

		Convey("When symbols are provided", func() {
			req := NewTradesHistoryRequest([]string{"BTC/USD", "ETH/USD"})
			raw, err := sonic.Marshal(req)
			So(err, ShouldBeNil)

			var payload map[string]any
			So(sonic.Unmarshal(raw, &payload), ShouldBeNil)

			Convey("pair is comma-joined", func() {
				So(payload["pair"], ShouldEqual, "BTC/USD,ETH/USD")
			})
		})
	})
}

func TestUnmarshalJSONHistoryTrade(t *testing.T) {
	Convey("Setup", t, func() {
		raw := []byte(`{
			"ordertxid":"O1",
			"postxid":"P1",
			"pair":"BTC/USD",
			"type":"buy",
			"ordertype":"market",
			"price":"50000",
			"cost":"100",
			"fee":"0.2",
			"vol":"0.002",
			"time":1700000000.0
		}`)
		var trade HistoryTrade

		Convey("When HistoryTrade unmarshals REST time as unix float", func() {
			So(sonic.Unmarshal(raw, &trade), ShouldBeNil)

			Convey("fields and UTC time decode", func() {
				So(trade.OrderID, ShouldEqual, "O1")
				So(trade.PositionID, ShouldEqual, "P1")
				So(trade.Pair, ShouldEqual, "BTC/USD")
				So(trade.Type, ShouldEqual, "buy")
				So(trade.Price.String(), ShouldEqual, "50000")
				So(trade.Volume.String(), ShouldEqual, "0.002")
				So(trade.Time, ShouldEqual, time.Unix(1700000000, 0).UTC())
			})
		})
	})
}

func TestUnmarshalJSONTradesHistory(t *testing.T) {
	Convey("Setup", t, func() {
		history := NewTradesHistory()
		called := 0
		history.OnChange(func(_ market.MarketType) { called++ })

		raw := []byte(`{
			"error":[],
			"result":{
				"count":"1",
				"trades":{
					"T1":{
						"ordertxid":"O1",
						"pair":"BTC/USD",
						"type":"sell",
						"time":1700000001.0
					}
				},
				"cursor":{"next":"abc"}
			}
		}`)

		Convey("When TradesHistory REST JSON arrives", func() {
			So(history.UnmarshalJSON(raw), ShouldBeNil)

			Convey("trades map and OnChange fire", func() {
				So(called, ShouldEqual, 1)
				So(history.Result.Trades["T1"].Pair, ShouldEqual, "BTC/USD")
				So(history.Result.Cursor.Next, ShouldEqual, "abc")
			})
		})

		Convey("When the venue returns an error array", func() {
			bad := []byte(`{"error":["EGeneral:Internal error"],"result":{}}`)
			err := history.UnmarshalJSON(bad)

			Convey("UnmarshalJSON surfaces the error", func() {
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func TestNewTradesHistoryResult(t *testing.T) {
	Convey("Setup", t, func() {
		raw := []byte(`{
			"error":[],
			"result":{
				"count":"2",
				"trades":{
					"T1":{"pair":"BTC/USD","time":1},
					"T2":{"pair":"ETH/USD","time":2}
				}
			}
		}`)

		Convey("When NewTradesHistoryResult parses the envelope", func() {
			result := NewTradesHistoryResult(raw)

			Convey("trades map length matches count", func() {
				So(result, ShouldNotBeNil)
				So(len(result.Trades), ShouldEqual, 2)
				So(result.Trades["T1"].Pair, ShouldEqual, "BTC/USD")
				So(result.Trades["T2"].Pair, ShouldEqual, "ETH/USD")
			})
		})
	})
}

func TestNewTradesHistoryFromMap(t *testing.T) {
	Convey("Setup", t, func() {
		model := map[string]any{
			"trades": []any{
				map[string]any{
					"id":       "T1",
					"order_id": "O1",
					"pair":     "BTC/USD",
					"side":     "buy",
					"price":    100.0,
					"cost":     100.0,
					"fee":      0.1,
					"volume":   1.0,
					"time":     1700000000.0,
				},
			},
		}

		Convey("When NewTradesHistoryFromMap adapts paper CLI rows", func() {
			out := NewTradesHistoryFromMap(model).(*TradesHistoryResult)

			Convey("trade fields land on HistoryTrade", func() {
				So(out.Count.String(), ShouldEqual, "1")
				trade := out.Trades["T1"]
				So(trade.OrderID, ShouldEqual, "O1")
				So(trade.Pair, ShouldEqual, "BTC/USD")
				So(trade.Type, ShouldEqual, "buy")
				So(trade.Price.String(), ShouldEqual, "100")
				So(trade.Volume.String(), ShouldEqual, "1")
				So(trade.Time, ShouldEqual, time.Unix(1700000000, 0).UTC())
			})
		})
	})
}
