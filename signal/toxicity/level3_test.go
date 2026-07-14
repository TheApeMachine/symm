package toxicity

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
)

func TestLevel3On(t *testing.T) {
	Convey("Given one Kraken L3 update", t, func() {
		level3 := &Level3{cache: []kraken.Level3Data{}}
		raw := []byte(`{
			"channel":"level3",
			"type":"update",
			"data":[{
				"symbol":"BTC/USD",
				"timestamp":"2026-07-14T09:00:00Z",
				"bids":[{
					"event":"add",
					"order_id":"bid-1",
					"limit_price":"100.0",
					"order_qty":"2.0",
					"timestamp":"2026-07-14T09:00:00Z"
				}]
			}]
		}`)

		level3.On(raw)
		rows := level3.Rows()

		Convey("Then the row is transferred once with its frame type", func() {
			So(rows, ShouldHaveLength, 1)
			So(rows[0].Symbol, ShouldEqual, "BTC/USD")
			So(rows[0].Type, ShouldEqual, "update")
			So(rows[0].Bids, ShouldHaveLength, 1)
			So(level3.Rows(), ShouldBeEmpty)
		})
	})
}

func BenchmarkLevel3On(b *testing.B) {
	raw := []byte(`{
		"channel":"level3",
		"type":"update",
		"data":[{
			"symbol":"BTC/USD",
			"timestamp":"2026-07-14T09:00:00Z",
			"bids":[{
				"event":"add",
				"order_id":"bid-1",
				"limit_price":"100.0",
				"order_qty":"2.0",
				"timestamp":"2026-07-14T09:00:00Z"
			}]
		}]
	}`)
	level3 := &Level3{cache: []kraken.Level3Data{}}
	b.ReportAllocs()

	for b.Loop() {
		level3.On(raw)
		level3.Rows()
	}
}
