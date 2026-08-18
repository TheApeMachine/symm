package hindsight

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

var epoch = time.Unix(0, 0).UTC()

/*
seriesFromPrices builds a Series from bare prices with synthetic strictly
increasing timestamps, so the greedy and swing decompositions can be asserted
on their shape without noise from wall times.
*/
func seriesFromPrices(prices ...float64) *Series {
	points := make([]Point, 0, len(prices))

	for index, price := range prices {
		points = append(points, Point{
			At:    epoch.Add(time.Duration(index) * time.Second),
			Price: price,
			Qty:   1,
		})
	}

	return &Series{Symbol: "TEST/USD", Points: points}
}

func TestIngest(t *testing.T) {
	Convey("Given a trade update frame", t, func() {
		payload := []byte(`{"channel":"trade","type":"update","data":[
			{"symbol":"BTC/USD","side":"buy","price":100,"qty":1,"timestamp":"2026-01-01T00:00:01Z"},
			{"symbol":"BTC/USD","side":"sell","price":102,"qty":2,"timestamp":"2026-01-01T00:00:02Z"},
			{"symbol":"ETH/USD","side":"buy","price":50,"qty":5,"timestamp":"2026-01-01T00:00:03Z"}
		]}`)
		reducer := NewReducer()

		Convey("Ingest should record every symbol with its venue time", func() {
			err := reducer.Ingest(payload, epoch)
			So(err, ShouldBeNil)

			btc := reducer.SeriesFor("BTC/USD")
			So(btc, ShouldNotBeNil)
			So(len(btc.Points), ShouldEqual, 2)
			So(btc.Points[0].Price, ShouldEqual, 100)
			So(btc.Points[0].At, ShouldEqual, time.Date(2026, 1, 1, 0, 0, 1, 0, time.UTC))
			So(btc.Points[1].Price, ShouldEqual, 102)
			So(btc.Points[1].Qty, ShouldEqual, 2)

			eth := reducer.SeriesFor("ETH/USD")
			So(eth, ShouldNotBeNil)
			So(len(eth.Points), ShouldEqual, 1)
		})
	})

	Convey("Given a non-trade frame", t, func() {
		reducer := NewReducer()

		Convey("Ingest should skip heartbeat and subscribe ack frames", func() {
			So(reducer.Ingest([]byte(`{"channel":"heartbeat","type":"update","data":[{}]}`), epoch), ShouldBeNil)
			So(reducer.Ingest([]byte(`{"method":"subscribe","result":{"channel":"trade","symbol":"BTC/USD"}}`), epoch), ShouldBeNil)
			So(len(reducer.Symbols()), ShouldEqual, 0)
		})
	})

	Convey("Given a trade frame with malformed rows", t, func() {
		reducer := NewReducer()

		Convey("Ingest should drop rows with zero or negative price or qty", func() {
			payload := []byte(`{"channel":"trade","type":"update","data":[
				{"symbol":"BTC/USD","side":"buy","price":0,"qty":1},
				{"symbol":"BTC/USD","side":"buy","price":100,"qty":0},
				{"symbol":"BTC/USD","side":"buy","price":-5,"qty":1},
				{"symbol":"","side":"buy","price":100,"qty":1}
			]}`)
			So(reducer.Ingest(payload, epoch), ShouldBeNil)
			So(len(reducer.Symbols()), ShouldEqual, 0)
		})
	})
}
