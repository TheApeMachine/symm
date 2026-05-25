package ui

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCandleStreamObserve(t *testing.T) {
	Convey("Given a source-side candle stream", t, func() {
		candleStream, err := NewCandleStream(5 * time.Second)
		So(err, ShouldBeNil)

		start := time.Unix(1_700_000_001, 0)
		first, err := candleStream.Observe("BTC/EUR", 100, start)
		So(err, ShouldBeNil)

		second, err := candleStream.Observe("BTC/EUR", 101, start.Add(time.Second))
		So(err, ShouldBeNil)

		third, err := candleStream.Observe("BTC/EUR", 99, start.Add(5*time.Second))
		So(err, ShouldBeNil)

		Convey("It should merge prices inside one candle bucket", func() {
			So(first.Sec, ShouldEqual, int64(1_700_000_000))
			So(second.Open, ShouldEqual, 100)
			So(second.High, ShouldEqual, 101)
			So(second.Low, ShouldEqual, 100)
			So(second.Close, ShouldEqual, 101)
		})

		Convey("It should start a new bar when the bucket advances", func() {
			So(third.Sec, ShouldEqual, int64(1_700_000_005))
			So(third.Open, ShouldEqual, 99)
			So(third.High, ShouldEqual, 99)
			So(third.Low, ShouldEqual, 99)
			So(third.Close, ShouldEqual, 99)
		})
	})
}

func TestCandleStreamObserveRejectsInvalidInput(t *testing.T) {
	Convey("Given a source-side candle stream", t, func() {
		candleStream, err := NewCandleStream(5 * time.Second)
		So(err, ShouldBeNil)

		_, err = candleStream.Observe("BTC/EUR", 0, time.Now())

		Convey("It should reject invalid prices", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func TestCandleStreamObserveTicker(t *testing.T) {
	Convey("Given a Kraken ticker timestamp", t, func() {
		candleStream, err := NewCandleStream(5 * time.Second)
		So(err, ShouldBeNil)

		bar, err := candleStream.ObserveTicker(
			"BTC/EUR",
			100,
			"2026-05-25T12:00:01.000000000Z",
		)

		Convey("It should parse and bucket the ticker update", func() {
			So(err, ShouldBeNil)
			So(bar.Sec, ShouldEqual, int64(1_779_710_400))
		})
	})
}

func BenchmarkCandleStreamObserve(benchmark *testing.B) {
	candleStream, err := NewCandleStream(5 * time.Second)

	if err != nil {
		benchmark.Fatal(err)
	}

	start := time.Unix(1_700_000_000, 0)

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		if _, err := candleStream.Observe("BTC/EUR", 100, start); err != nil {
			benchmark.Fatal(err)
		}
	}
}
