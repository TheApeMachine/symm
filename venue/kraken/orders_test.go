package kraken

import (
	"testing"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	. "github.com/smartystreets/goconvey/convey"
)

func openOrdersFromWire(data []byte) map[string]HistoryOrder {
	var envelope OpenOrders

	if err := sonic.Unmarshal(data, &envelope); err != nil || len(envelope.Error) > 0 {
		return nil
	}

	if envelope.Result.Open == nil {
		return map[string]HistoryOrder{}
	}

	return envelope.Result.Open
}

func closedOrdersFromWire(data []byte) map[string]HistoryOrder {
	var envelope ClosedOrders

	if err := sonic.Unmarshal(data, &envelope); err != nil || len(envelope.Error) > 0 {
		return nil
	}

	if envelope.Result.Closed == nil {
		return map[string]HistoryOrder{}
	}

	return envelope.Result.Closed
}

func TestNewQueryOrdersRequest(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When NewQueryOrdersRequest joins ids", func() {
			So(NewQueryOrdersRequest([]string{"o1", "o2"}).Txid, ShouldEqual, "o1,o2")
		})
	})
}

func TestHistoryOrderRequest(t *testing.T) {
	Convey("Setup", t, func() {
		vol := decimal.RequireFromString("1.5")
		order := HistoryOrder{
			ClOrdID: "enter-1",
			Vol:     &vol,
			Descr: HistoryOrderDescr{
				Pair:      "XBTUSD",
				Type:      "buy",
				OrderType: "market",
			},
		}

		Convey("When Request projects the spot OrderRequest", func() {
			req := order.Request()

			Convey("Wire fields match the venue row", func() {
				So(req.ClOrdId, ShouldEqual, "enter-1")
				So(req.Type, ShouldEqual, "buy")
				So(req.OrderType, ShouldEqual, "market")
				So(req.Volume, ShouldEqual, "1.5")
			})
		})
	})
}

func TestNewQueryOrdersResult(t *testing.T) {
	Convey("Setup", t, func() {
		raw := []byte(`{
			"error": [],
			"result": {
				"o1": {
					"cl_ord_id": "enter-1",
					"status": "closed",
					"vol": "1.0",
					"vol_exec": "1.0",
					"cost": "100.0",
					"fee": "0.1",
					"price": "100.0",
					"opentm": 1700000000.5,
					"descr": {
						"pair": "XBTUSD",
						"type": "buy",
						"ordertype": "market",
						"price": "0"
					}
				}
			}
		}`)

		Convey("When NewQueryOrdersResult parses the envelope", func() {
			orders := NewQueryOrdersResult(raw)

			Convey("Orders are keyed by txid with decimal volumes", func() {
				So(orders, ShouldNotBeNil)
				So(len(orders), ShouldEqual, 1)
				So(orders["o1"].ClOrdID, ShouldEqual, "enter-1")
				So(orders["o1"].Descr.Type, ShouldEqual, "buy")
				So(orders["o1"].Vol.String(), ShouldEqual, "1")
				So(orders["o1"].OpenTm.String(), ShouldEqual, "1700000000.5")
			})
		})
	})
}

func TestNewOpenOrdersResult(t *testing.T) {
	Convey("Setup", t, func() {
		raw := []byte(`{
			"error": [],
			"result": {
				"open": {
					"OQCLML-BW3P3-BUCMWZ": {
						"refid": "None",
						"userref": 0,
						"status": "open",
						"opentm": 1688666559.8974,
						"starttm": 0,
						"expiretm": 0,
						"descr": {
							"pair": "XBTUSD",
							"type": "buy",
							"ordertype": "limit",
							"price": "30010.0",
							"price2": "0",
							"leverage": "none",
							"order": "buy 1.25000000 XBTUSD @ limit 30010.0",
							"close": ""
						},
						"vol": "1.25000000",
						"vol_exec": "0.37500000",
						"cost": "11253.7",
						"fee": "0.00000",
						"price": "30010.0",
						"stopprice": "0.00000",
						"limitprice": "0.00000",
						"misc": "",
						"oflags": "fciq",
						"trades": ["TCCCTY-WE2O6-P3NB37"]
					},
					"OB5VMB-B4U2U-DK2WRW": {
						"refid": "None",
						"userref": 45326,
						"status": "open",
						"opentm": 1688665899.5699,
						"starttm": 0,
						"expiretm": 0,
						"descr": {
							"pair": "XBTUSD",
							"type": "buy",
							"ordertype": "limit",
							"price": "14500.0",
							"price2": "0",
							"leverage": "5:1",
							"order": "buy 0.27500000 XBTUSD @ limit 14500.0 with 5:1 leverage",
							"close": ""
						},
						"vol": "0.27500000",
						"vol_exec": "0.00000000",
						"cost": "0.00000",
						"fee": "0.00000",
						"price": "0.00000",
						"stopprice": "0.00000",
						"limitprice": "0.00000",
						"misc": "",
						"oflags": "fciq"
					}
				}
			}
		}`)

		Convey("When openOrdersFromWire parses the sample envelope", func() {
			orders := openOrdersFromWire(raw)

			Convey("Both open orders decode with decimal fields", func() {
				So(orders, ShouldNotBeNil)
				So(len(orders), ShouldEqual, 2)

				partial := orders["OQCLML-BW3P3-BUCMWZ"]
				So(partial.Status, ShouldEqual, "open")
				So(partial.Vol.String(), ShouldEqual, "1.25")
				So(partial.VolExec.String(), ShouldEqual, "0.375")
				So(partial.Cost.String(), ShouldEqual, "11253.7")
				So(partial.Descr.Price.String(), ShouldEqual, "30010")
				So(partial.Descr.Leverage, ShouldEqual, "none")
				So(partial.OFlags, ShouldEqual, "fciq")
				So(partial.Trades, ShouldResemble, []string{"TCCCTY-WE2O6-P3NB37"})

				levered := orders["OB5VMB-B4U2U-DK2WRW"]
				So(levered.UserRef, ShouldEqual, 45326)
				So(levered.Descr.Leverage, ShouldEqual, "5:1")
				So(levered.Vol.String(), ShouldEqual, "0.275")
				So(levered.VolExec.String(), ShouldEqual, "0")
			})
		})
	})
}

func TestNewClosedOrdersResult(t *testing.T) {
	Convey("Setup", t, func() {
		raw := []byte(`{
			"error": [],
			"result": {
				"closed": {
					"O37652-RJWRT-IMO74O": {
						"refid": "None",
						"userref": 1,
						"status": "canceled",
						"reason": "User requested",
						"opentm": 1688148493.7708,
						"closetm": 1688148610.0482,
						"starttm": 0,
						"expiretm": 0,
						"descr": {
							"pair": "XBTGBP",
							"type": "buy",
							"ordertype": "stop-loss-limit",
							"price": "23667.0",
							"price2": "0",
							"leverage": "none",
							"order": "buy 0.00100000 XBTGBP @ limit 23667.0",
							"close": ""
						},
						"vol": "0.00100000",
						"vol_exec": "0.00000000",
						"cost": "0.00000",
						"fee": "0.00000",
						"price": "0.00000",
						"stopprice": "0.00000",
						"limitprice": "0.00000",
						"misc": "",
						"oflags": "fciq",
						"trigger": "index"
					}
				},
				"count": 1
			}
		}`)

		Convey("When closedOrdersFromWire parses the envelope", func() {
			orders := closedOrdersFromWire(raw)

			Convey("Closed order is restored", func() {
				So(orders, ShouldNotBeNil)
				So(len(orders), ShouldEqual, 1)
				order := orders["O37652-RJWRT-IMO74O"]
				So(order.Status, ShouldEqual, "canceled")
				So(order.Reason, ShouldEqual, "User requested")
				So(order.Trigger, ShouldEqual, "index")
				So(order.Vol.String(), ShouldEqual, "0.001")
				So(order.CloseTm, ShouldNotBeNil)
			})
		})
	})
}
