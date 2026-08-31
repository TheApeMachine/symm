package hindsight

import (
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func indexSeries() []Observation {
	series := excursionSeries()

	for index := range series {
		if index%5 != 0 {
			continue
		}

		series[index].Symbol = "OTHER/USD"
	}

	return series
}

func TestRunIndexProjectTest(t *testing.T) {
	Convey("Given a run index over captured observations", t, func() {
		index := NewRunIndex("run-test", indexSeries())

		Convey("It groups every instrument the tape carried", func() {
			So(index.Observations(), ShouldEqual, 400)
			So(len(index.Summaries(DefaultDiscoveryPolicy())), ShouldEqual, 2)
		})

		Convey("The time axis positions buckets by the instants observed", func() {
			timeline := index.Project(TimelineRequest{
				Run:     "run-test",
				Symbol:  "TEST/USD",
				Axis:    AxisTime,
				Buckets: 16,
			})

			So(timeline.Axis, ShouldEqual, AxisTime)
			So(len(timeline.Buckets), ShouldEqual, 16)

			previous := time.Time{}

			for _, bucket := range timeline.Buckets {
				So(bucket.FromAt.Before(bucket.ToAt) || bucket.FromAt.Equal(bucket.ToAt), ShouldBeTrue)
				So(bucket.FromAt.Before(previous), ShouldBeFalse)
				previous = bucket.FromAt
			}
		})

		Convey("The capture axis positions buckets by observation order", func() {
			timeline := index.Project(TimelineRequest{
				Run:     "run-test",
				Symbol:  "TEST/USD",
				Axis:    AxisCapture,
				Buckets: 16,
			})

			So(timeline.Axis, ShouldEqual, AxisCapture)

			previous := CaptureSequence(0)

			for _, bucket := range timeline.Buckets {
				So(bucket.FromSeq, ShouldBeGreaterThan, previous)
				previous = bucket.FromSeq
			}
		})

		Convey("Every bucket resolves to captures that were actually observed", func() {
			timeline := index.Project(TimelineRequest{
				Run:     "run-test",
				Symbol:  "TEST/USD",
				Axis:    AxisTime,
				Buckets: 24,
			})

			for _, bucket := range timeline.Buckets {
				if bucket.Observations == 0 {
					continue
				}

				So(bucket.ObservedFromSeq, ShouldBeGreaterThan, CaptureSequence(0))
				So(bucket.ObservedToSeq, ShouldBeGreaterThanOrEqualTo, bucket.ObservedFromSeq)
			}
		})

		Convey("Resolution never exceeds the observations it would render", func() {
			timeline := index.Project(TimelineRequest{
				Run:     "run-test",
				Symbol:  "TEST/USD",
				Axis:    AxisTime,
				Buckets: 100000,
			})

			So(len(timeline.Buckets), ShouldBeLessThanOrEqualTo, timeline.Discovery.Observations)
		})

		Convey("An instrument that was never captured projects nothing, not zeros", func() {
			timeline := index.Project(TimelineRequest{
				Run:     "run-test",
				Symbol:  "ABSENT/USD",
				Buckets: 16,
			})

			So(timeline.Buckets, ShouldBeEmpty)
			So(timeline.Discovery.Episodes, ShouldBeEmpty)
		})

		Convey("The instrument index is answered only when it is asked for", func() {
			without := index.Project(TimelineRequest{Run: "run-test", Symbol: "TEST/USD"})
			So(without.Symbols, ShouldBeEmpty)

			with := index.Project(TimelineRequest{
				Run:     "run-test",
				Symbol:  "TEST/USD",
				Symbols: true,
			})
			So(with.Symbols, ShouldNotBeEmpty)
		})
	})
}

func TestRunIndexStreamSpansTest(t *testing.T) {
	Convey("Given captured observations spanning a reconnect", t, func() {
		series := excursionSeries()

		for index := range series {
			if index < 200 {
				continue
			}

			series[index].Capture.StreamEpoch = 2
		}

		index := NewRunIndex("run-test", series)
		timeline := index.Project(TimelineRequest{Run: "run-test", Symbol: "TEST/USD"})

		Convey("The second epoch of the stream is recorded as a reconnect", func() {
			So(len(timeline.Streams), ShouldEqual, 2)
			So(timeline.Streams[0].Reconnect, ShouldBeFalse)
			So(timeline.Streams[1].Reconnect, ShouldBeTrue)
			So(timeline.Streams[1].Epoch, ShouldEqual, StreamEpoch(2))
			So(timeline.Streams[1].FromSeq, ShouldBeGreaterThan, timeline.Streams[0].FromSeq)
		})
	})
}

/*
TestRunIndexConcurrentReadersTest holds the contract that one assembled index
serves every concurrent reader of a run. The memo it fills in as it answers is
shared state on a server that handles requests in parallel; run with -race, an
unguarded map here is a fatal, not a flake.
*/
func TestRunIndexConcurrentReadersTest(t *testing.T) {
	Convey("Given one index serving many readers at once", t, func() {
		index := NewRunIndex("run-test", indexSeries())
		coordinates := []MarketCoordinate{
			CoordinateMidpoint,
			CoordinateBid,
			CoordinateAsk,
			CoordinateLast,
		}

		var waiting sync.WaitGroup

		for worker := range 32 {
			waiting.Add(1)

			go func(worker int) {
				defer waiting.Done()

				policy := DefaultDiscoveryPolicy()
				policy.Coordinate = coordinates[worker%len(coordinates)]

				index.Project(TimelineRequest{
					Run:     "run-test",
					Symbol:  "TEST/USD",
					Policy:  policy,
					Buckets: 16,
					Symbols: true,
				})
			}(worker)
		}

		waiting.Wait()

		Convey("Every reader is answered without corrupting the shared memo", func() {
			discovery := index.Discover("TEST/USD", DefaultDiscoveryPolicy())
			So(discovery.Symbol, ShouldEqual, "TEST/USD")
			So(discovery.Coordinate, ShouldEqual, CoordinateMidpoint)
		})
	})
}
