package data

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

func observe[Value any](
	t *testing.T,
	node core.Primitive,
	input SeriesInput[Value],
) SeriesReading[Value] {
	t.Helper()

	readingEval := transport.NewEvaluate(node)
	var reading SeriesReading[Value]

	for out := range readingEval.Next(transport.NewValues(input).Next(nil)) {
		reading = *(*SeriesReading[Value])(out)
	}

	err := readingEval.Error()
	So(err, ShouldBeNil)
	return reading
}

func TestSeriesNext(t *testing.T) {
	Convey("Given out-of-order observations inside a bounded series", t, func() {
		node := NewSeries[[2]float64](3)
		base := 1_700_008_000.0

		So(observe(t, node, SeriesInput[[2]float64]{Key: "one", Sec: base + 2, Value: [2]float64{102, 103}}).Found, ShouldBeTrue)
		So(observe(t, node, SeriesInput[[2]float64]{Key: "one", Sec: base, Value: [2]float64{100, 101}}).Found, ShouldBeTrue)
		So(observe(t, node, SeriesInput[[2]float64]{Key: "one", Sec: base + 1, Value: [2]float64{101, 102}}).Found, ShouldBeTrue)

		Convey("It should retain every observation", func() {
			reading := observe(t, node, SeriesInput[[2]float64]{Key: "one", Sec: base + 2, Query: true})

			So(reading.Found, ShouldBeTrue)
			So(reading.Value, ShouldResemble, [2]float64{102, 103})
		})

		Convey("It should replace a re-observed event time in place", func() {
			So(observe(t, node, SeriesInput[[2]float64]{Key: "one", Sec: base + 1, Value: [2]float64{101.5, 102.5}}).Found, ShouldBeTrue)

			reading := observe(t, node, SeriesInput[[2]float64]{Key: "one", Sec: base + 1, Query: true})

			So(reading.Found, ShouldBeTrue)
			So(reading.Value, ShouldResemble, [2]float64{101.5, 102.5})
		})
	})

	Convey("Given observations on unnormalized clock coordinates", t, func() {
		node := NewSeries[[2]float64](3)
		base := 1_700_008_000.0

		Convey("It should refuse them without failing", func() {
			So(observe(t, node, SeriesInput[[2]float64]{Key: "one", Sec: base, Nsec: 1e9, Value: [2]float64{100, 101}}).Found, ShouldBeFalse)
			So(observe(t, node, SeriesInput[[2]float64]{Key: "one", Sec: base, Nsec: -1, Value: [2]float64{100, 101}}).Found, ShouldBeFalse)
			So(observe(t, node, SeriesInput[[2]float64]{Key: "", Sec: base, Value: [2]float64{100, 101}}).Found, ShouldBeFalse)

			So(observe(t, node, SeriesInput[[2]float64]{Key: "one", Sec: base, Query: true}).Found, ShouldBeFalse)
		})

		Convey("It should accept any normalized clock, including epoch-relative times", func() {
			So(observe(t, node, SeriesInput[[2]float64]{Key: "one", Nsec: 5e8, Value: [2]float64{100, 101}}).Found, ShouldBeTrue)

			reading := observe(t, node, SeriesInput[[2]float64]{Key: "one", Sec: 0, Nsec: 6e8, Query: true})

			So(reading.Found, ShouldBeTrue)
			So(reading.Value, ShouldResemble, [2]float64{100, 101})
		})
	})

	Convey("Given a non-positive series capacity", t, func() {
		node := NewSeries[[2]float64](0)
		So(node.Error(), ShouldNotBeNil)
	})
}

func TestSeriesAsOf(t *testing.T) {
	Convey("Given out-of-order observations inside a bounded series", t, func() {
		node := NewSeries[[2]float64](3)
		base := 1_700_008_000.0

		for _, input := range []SeriesInput[[2]float64]{
			{Key: "one", Sec: base + 2, Value: [2]float64{102, 103}},
			{Key: "one", Sec: base, Value: [2]float64{100, 101}},
			{Key: "one", Sec: base + 1, Value: [2]float64{101, 102}},
		} {
			So(observe(t, node, input).Found, ShouldBeTrue)
		}

		Convey("It should select the newest value no later than the event", func() {
			reading := observe(t, node, SeriesInput[[2]float64]{Key: "one", Sec: base + 1, Nsec: 5e8, Query: true})

			So(reading.Found, ShouldBeTrue)
			So(reading.Value, ShouldResemble, [2]float64{101, 102})
		})

		Convey("It should never explain an older event with a later value", func() {
			So(observe(t, node, SeriesInput[[2]float64]{Key: "one", Sec: base - 1, Query: true}).Found, ShouldBeFalse)
		})

		Convey("It should never answer for a key it has not observed", func() {
			So(observe(t, node, SeriesInput[[2]float64]{Key: "two", Sec: base, Query: true}).Found, ShouldBeFalse)
		})
	})

	Convey("Given a ring that overflowed its capacity", t, func() {
		node := NewSeries[[2]float64](3)
		base := 1_700_008_000.0

		for offset := range 4 {
			So(observe(t, node, SeriesInput[[2]float64]{
				Key: "one", Sec: base + float64(offset),
				Value: [2]float64{100 + float64(offset), 101 + float64(offset)},
			}).Found, ShouldBeTrue)
		}

		Convey("It should answer from the retained window only", func() {
			So(observe(t, node, SeriesInput[[2]float64]{Key: "one", Sec: base, Query: true}).Found, ShouldBeFalse)

			reading := observe(t, node, SeriesInput[[2]float64]{Key: "one", Sec: base + 1, Query: true})

			So(reading.Found, ShouldBeTrue)
			So(reading.Value, ShouldResemble, [2]float64{101, 102})
		})
	})
}

func BenchmarkSeriesNext(b *testing.B) {
	node := NewSeries[[2]float64](128)
	base := 1_700_008_000.0
	b.ReportAllocs()

	for b.Loop() {
		for range node.Next(transport.NewValues(SeriesInput[[2]float64]{
			Key: "one", Sec: base, Value: [2]float64{100, 101},
		}, SeriesInput[[2]float64]{Key: "one", Sec: base, Query: true}).Next(nil)) {
		}
	}
}
