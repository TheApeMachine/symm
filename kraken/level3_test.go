package kraken

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bytedance/sonic"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNewLevel3DataSlice(t *testing.T) {
	Convey("Given Kraken level3 data payloads", t, func() {
		payload := []byte(`[{
			"symbol": "BTC/USD",
			"checksum": 291736120,
			"bids": [{
				"event": "add",
				"order_id": "OQCLML-BW3P3-BUCMWZ",
				"limit_price": 43125.3,
				"order_qty": 0.15,
				"timestamp": "2022-12-25T09:30:59.123456789Z"
			}],
			"asks": []
		}]`)

		levels := NewLevel3DataSlice(payload)

		Convey("It should decode order identity, price, and quantity", func() {
			So(len(levels), ShouldEqual, 1)

			level3 := levels[0]

			So(level3.Symbol, ShouldEqual, "BTC/USD")
			So(level3.Checksum, ShouldEqual, uint32(291736120))
			So(level3.Bids[0].OrderID, ShouldEqual, "OQCLML-BW3P3-BUCMWZ")
			So(level3.Bids[0].LimitPrice.String(), ShouldEqual, "43125.3")
			So(level3.Bids[0].OrderQty.String(), ShouldEqual, "0.15")
		})
	})
}

func TestLevel3OrderUnmarshalJSON(t *testing.T) {
	Convey("Given Kraken fixed-point level3 decimals with significant trailing zeroes", t, func() {
		payload := []byte(`{
			"order_id":"order-1",
			"limit_price":43125.300,
			"order_qty":"0.15000000",
			"timestamp":"2026-07-12T00:00:00Z"
		}`)
		order := Level3Order{}

		Convey("When the order is decoded", func() {
			err := sonic.Unmarshal(payload, &order)

			Convey("Then calculation values and checksum text remain distinct and exact", func() {
				So(err, ShouldBeNil)
				So(order.LimitPrice.String(), ShouldEqual, "43125.300")
				So(order.OrderQty.String(), ShouldEqual, "0.15000000")
				So(order.ChecksumLimitPrice(), ShouldEqual, "43125.300")
				So(order.ChecksumOrderQty(), ShouldEqual, "0.15000000")
			})
		})
	})

	Convey("Given a valid decimal expressed in scientific notation", t, func() {
		order := Level3Order{}

		Convey("When the order is decoded", func() {
			err := sonic.Unmarshal([]byte(`{
				"order_id":"order-1",
				"limit_price":1e2,
				"order_qty":1
			}`), &order)

			Convey("Then it is canonicalized to deterministic fixed-point checksum text", func() {
				So(err, ShouldBeNil)
				So(order.LimitPrice.String(), ShouldEqual, "100")
				So(order.ChecksumLimitPrice(), ShouldEqual, "100")
			})
		})
	})
}

func BenchmarkNewLevel3(b *testing.B) {
	var builder strings.Builder
	builder.WriteString(`{"channel":"level3","type":"snapshot","data":[{"symbol":"BTC/USD","checksum":0,"bids":[`)

	for index := range 25 {
		if index > 0 {
			builder.WriteByte(',')
		}

		_, _ = fmt.Fprintf(
			&builder,
			`{"order_id":"bid-%d","limit_price":%.3f,"order_qty":%.8f,"timestamp":"2026-07-12T00:00:00Z"}`,
			index,
			43125.300-float64(index),
			0.15+float64(index)/100,
		)
	}

	builder.WriteString(`],"asks":[`)

	for index := range 25 {
		if index > 0 {
			builder.WriteByte(',')
		}

		_, _ = fmt.Fprintf(
			&builder,
			`{"order_id":"ask-%d","limit_price":%.3f,"order_qty":%.8f,"timestamp":"2026-07-12T00:00:00Z"}`,
			index,
			43125.400+float64(index),
			0.15+float64(index)/100,
		)
	}

	builder.WriteString(`]}]}`)
	payload := []byte(builder.String())
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		level3 := NewLevel3(payload)

		if len(level3.Data) != 1 || len(level3.Data[0].Bids) != 25 || len(level3.Data[0].Asks) != 25 {
			b.Fatal("incomplete level3 decode")
		}
	}
}
