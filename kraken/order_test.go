package kraken

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNewOrderDataSliceFromSpot(testingTB *testing.T) {
	Convey("Given spot REST open orders", testingTB, func() {
		openTime, err := decimal.NewFromString("1780000000.0")
		So(err, ShouldBeNil)
		volume, err := decimal.NewFromString("0.2")
		So(err, ShouldBeNil)
		executed, err := decimal.NewFromString("0.05")
		So(err, ShouldBeNil)
		price, err := decimal.NewFromString("100000")
		So(err, ShouldBeNil)

		orders := map[string]spot.Order{
			"O1": {
				OpenTm:         openTime,
				Volume:         volume,
				VolumeExecuted: executed,
				Description: &spot.OrderDescription{
					Pair:      "BTC/USD",
					Type:      "buy",
					OrderType: "limit",
					Price:     price,
				},
			},
		}
		rows := NewOrderDataSliceFromSpot(orders)

		Convey("Then they should become private order rows", func() {
			So(rows, ShouldHaveLength, 1)
			So(rows[0].ID, ShouldEqual, "O1")
			So(rows[0].Pair, ShouldEqual, "BTC/USD")
			So(rows[0].Side, ShouldEqual, "buy")
			So(rows[0].Type, ShouldEqual, "limit")
			So(rows[0].Volume.String(), ShouldEqual, "0.15")
			So(rows[0].ReservedAsset, ShouldEqual, "USD")
			So(rows[0].ReservedAmount.String(), ShouldEqual, "15000.00")
			So(rows[0].CreatedAt, ShouldNotBeBlank)
		})
	})
}

func TestNewOrderResponse(testingTB *testing.T) {
	Convey("Given a Kraken websocket add_order response", testingTB, func() {
		buf := []byte(`{
			"method": "add_order",
			"result": {
				"order_id": "OK4GJX-KSTLS-7DZZO5",
				"order_userref": 3,
				"warnings": ["post only ignored"]
			},
			"success": true,
			"req_id": 123456789,
			"time_in": "2022-12-25T09:30:59.123456Z",
			"time_out": "2022-12-25T09:30:59.223456Z"
		}`)

		response := NewOrderResponse(buf)

		Convey("Then it should decode the documented response fields", func() {
			So(response.Method, ShouldEqual, "add_order")
			So(response.Success, ShouldBeTrue)
			So(response.ReqID, ShouldEqual, 123456789)
			So(response.Result.OrderID, ShouldEqual, "OK4GJX-KSTLS-7DZZO5")
			So(response.Result.OrderUserRef, ShouldEqual, 3)
			So(response.Result.Warnings, ShouldHaveLength, 1)
			So(response.TimeIn.IsZero(), ShouldBeFalse)
			So(response.TimeOut.IsZero(), ShouldBeFalse)
		})
	})
}

func BenchmarkNewOrderDataSlice(benchmarkTB *testing.B) {
	buf := []byte(`[{"id":"PAPER-00003","pair":"BTCUSD","price":90000,"reserved_amount":9,"reserved_asset":"USD","side":"buy","type":"limit","volume":0.0001,"created_at":"2026-07-06T10:00:00Z"}]`)

	benchmarkTB.ReportAllocs()
	for benchmarkTB.Loop() {
		_ = NewOrderDataSlice(buf)
	}
}

func BenchmarkNewOrderDataSliceFromSpot(benchmarkTB *testing.B) {
	openTime, _ := decimal.NewFromString("1780000000.0")
	volume, _ := decimal.NewFromString("0.2")
	price, _ := decimal.NewFromString("100000")

	orders := map[string]spot.Order{
		"O1": {
			OpenTm: openTime,
			Volume: volume,
			Description: &spot.OrderDescription{
				Pair:      "BTC/USD",
				Type:      "buy",
				OrderType: "limit",
				Price:     price,
			},
		},
	}

	benchmarkTB.ReportAllocs()
	for benchmarkTB.Loop() {
		_ = NewOrderDataSliceFromSpot(orders)
	}
}
