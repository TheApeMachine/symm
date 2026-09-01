package depthflow

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
)

var depthflowBaseTime = time.Unix(1_700_000_000, 0)

func depthflowOrder(event string, price float64, quantity float64) kraken.Level3Order {
	return kraken.Level3Order{
		Event:      event,
		LimitPrice: decimal.NewFromFloat64(price),
		OrderQty:   decimal.NewFromFloat64(quantity),
		Timestamp:  depthflowBaseTime,
	}
}

func depthflowMetric(measurement *data.Measurement[float64], name string) float64 {
	return measurement.Metrics[name].Raw
}

/*
TestLevel3Step verifies that Step emits only mutation-derived observations and
never accumulates an order book between messages.
*/
func TestLevel3Step(t *testing.T) {
	Convey("Given a streaming depth-flow signal", t, func() {
		level3 := NewLevel3()

		Convey("a snapshot is reduced to facts carried by that one message", func() {
			measurement := level3.Step(kraken.Level3Data{
				Symbol:    "BTC/USD",
				Type:      "snapshot",
				Timestamp: depthflowBaseTime,
				Bids: []kraken.Level3Order{
					depthflowOrder("", 99, 2),
					depthflowOrder("", 98, 1),
				},
				Asks: []kraken.Level3Order{
					depthflowOrder("", 101, 2),
				},
			})

			So(measurement, ShouldNotBeNil)
			So(measurement.Err, ShouldBeNil)
			So(depthflowMetric(measurement, "observed_notional:bid"), ShouldAlmostEqual, 296)
			So(depthflowMetric(measurement, "observed_notional:ask"), ShouldAlmostEqual, 202)
			So(depthflowMetric(measurement, "observed_notional_imbalance"), ShouldAlmostEqual, 94.0/498.0)
			So(depthflowMetric(measurement, "mutation_count:bid"), ShouldEqual, 2)
			So(depthflowMetric(measurement, "mutation_count:ask"), ShouldEqual, 1)
		})

		Convey("the next message does not inherit untouched orders", func() {
			level3.Step(kraken.Level3Data{
				Symbol:    "BTC/USD",
				Type:      "snapshot",
				Timestamp: depthflowBaseTime,
				Bids:      []kraken.Level3Order{depthflowOrder("", 99, 2)},
				Asks:      []kraken.Level3Order{depthflowOrder("", 101, 2)},
			})

			measurement := level3.Step(kraken.Level3Data{
				Symbol:    "BTC/USD",
				Type:      "update",
				Timestamp: depthflowBaseTime.Add(time.Second),
				Bids:      []kraken.Level3Order{depthflowOrder("add", 100, 1)},
			})

			So(measurement.Err, ShouldBeNil)
			So(depthflowMetric(measurement, "observed_notional:bid"), ShouldEqual, 100)
			So(depthflowMetric(measurement, "observed_notional:ask"), ShouldEqual, 0)
			So(depthflowMetric(measurement, "observed_notional"), ShouldEqual, 100)
			So(depthflowMetric(measurement, "add_notional:bid"), ShouldEqual, 100)
			So(depthflowMetric(measurement, "mutation_activity_imbalance"), ShouldEqual, 1)
			So(depthflowMetric(measurement, "observed_notional_rate"), ShouldEqual, 100)
		})

		Convey("modify and delete retain only facts the wire actually supplies", func() {
			measurement := level3.Step(kraken.Level3Data{
				Symbol:    "ETH/USD",
				Type:      "update",
				Timestamp: depthflowBaseTime,
				Bids: []kraken.Level3Order{
					depthflowOrder("modify", 50, 3),
					depthflowOrder("delete", 49, 0),
				},
				Asks: []kraken.Level3Order{
					depthflowOrder("delete", 51, 0),
				},
			})

			So(measurement.Err, ShouldBeNil)
			So(depthflowMetric(measurement, "modify_remaining_notional:bid"), ShouldEqual, 150)
			So(depthflowMetric(measurement, "delete_count:bid"), ShouldEqual, 1)
			So(depthflowMetric(measurement, "delete_count:ask"), ShouldEqual, 1)
			So(depthflowMetric(measurement, "observed_notional:bid"), ShouldEqual, 150)
			So(depthflowMetric(measurement, "observed_notional:ask"), ShouldEqual, 0)
		})

		Convey("malformed and unknown events are explicit measurement errors", func() {
			unknown := level3.Step(kraken.Level3Data{
				Symbol:    "SOL/USD",
				Timestamp: depthflowBaseTime,
				Bids:      []kraken.Level3Order{depthflowOrder("change", 10, 1)},
			})

			So(unknown.Err, ShouldNotBeNil)

			missingPrice := depthflowOrder("add", 10, 1)
			missingPrice.LimitPrice = nil
			malformed := level3.Step(kraken.Level3Data{
				Symbol:    "XRP/USD",
				Timestamp: depthflowBaseTime,
				Bids:      []kraken.Level3Order{missingPrice},
			})

			So(malformed.Err, ShouldNotBeNil)
		})

		Convey("an empty increment yields no observation", func() {
			So(level3.Step(kraken.Level3Data{Symbol: "ADA/USD"}), ShouldBeNil)
		})
	})
}

/*
BenchmarkLevel3Step measures the complete steady-state streaming path.
*/
func BenchmarkLevel3Step(b *testing.B) {
	level3 := NewLevel3()
	message := kraken.Level3Data{
		Symbol:    "BTC/USD",
		Type:      "update",
		Timestamp: depthflowBaseTime,
		Bids:      []kraken.Level3Order{depthflowOrder("add", 99, 2)},
		Asks:      []kraken.Level3Order{depthflowOrder("modify", 101, 2)},
	}

	level3.Step(message)
	b.ReportAllocs()
	

	for b.Loop() {
		message.Timestamp = message.Timestamp.Add(time.Nanosecond)
		level3.Step(message)
	}
}
