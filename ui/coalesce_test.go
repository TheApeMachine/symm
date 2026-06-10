package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCoalesceKey(t *testing.T) {
	Convey("Given ui frame types", t, func() {
		Convey("It should coalesce gauges by source", func() {
			key := coalesceKey("gauge", map[string]any{
				"source": "cvd",
			})

			So(key, ShouldEqual, "gauge:cvd")
		})

		Convey("It should coalesce marks by symbol", func() {
			key := coalesceKey("mark", map[string]any{
				"symbol": "BTC/USD",
			})

			So(key, ShouldEqual, "mark:BTC/USD")
		})

		Convey("It should keep unique keys for snapshots", func() {
			So(coalesceKey("manifold_snapshot", map[string]any{"rho": 1.0}), ShouldEqual, "manifold_snapshot")
			So(coalesceKey("field_snapshot", map[string]any{"re": 1.0}), ShouldEqual, "field_snapshot")
		})
	})
}

func TestFrontendClientEnqueueLossy(t *testing.T) {
	Convey("Given a full outbound queue", t, func() {
		client := &frontendClient{
			outbound: make(chan []byte, 1),
		}

		client.enqueue([]byte(`{"first":true}`))
		client.enqueue([]byte(`{"second":true}`))

		Convey("It should keep the client connected and retain the latest frame", func() {
			So(client.closed.Load(), ShouldBeFalse)
			So(len(client.outbound), ShouldEqual, 1)
			So(string(<-client.outbound), ShouldEqual, `{"second":true}`)
		})
	})
}
