package adaptive_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestPathNext(t *testing.T) {
	Convey("Given a path with observation-driven support retention", t, func() {
		window := adaptive.NewWindow()
		path := adaptive.NewPath(window)
		expected := adaptive.NewWindow()
		retained := []float64{}
		var fields map[string]core.Primitive
		var first map[string]core.Primitive
		shed := false
		var at int64

		// Stable observations, a rising regime, a reversal and a stable plateau
		// force both support growth and change-driven reductions.
		for phase, level := range []float64{100, 1000, 10, 500} {
			for index := range 64 {
				at++
				value := level + float64(index%3)
				reading := expected.Observe(value)
				retained = append(retained, value)

				if len(retained) > int(reading.Capacity) {
					retained = retained[len(retained)-int(reading.Capacity):]
					shed = true
				}
				var err error
				fields, err = transport.Evaluate[map[string]core.Primitive](path, core.Record(map[string]any{"at": at, "value": value}))
				So(err, ShouldBeNil)
				observations, err := core.Field[[]core.Primitive](fields, "observations")
				So(err, ShouldBeNil)
				So(len(observations), ShouldEqual, len(retained))

				for offset, observation := range observations {
					actual, err := core.Field[float64](core.To[map[string]core.Primitive](observation), "value")
					So(err, ShouldBeNil)
					So(actual, ShouldEqual, retained[offset])
				}

				if phase == 0 && index == 0 {
					first = fields
				}
			}
		}
		So(shed, ShouldBeTrue)
		So(core.To[float64](first["count"]), ShouldEqual, 1)

		Convey("A regressed event does not advance the retention policy or edit history", func() {
			before := window.Reading
			regressed, err := transport.Evaluate[map[string]core.Primitive](path, core.Record(map[string]any{"at": at - 1, "value": -1000.0}))
			So(err, ShouldBeNil)
			So(core.To[bool](regressed["accepted"]), ShouldBeFalse)
			So(window.Reading, ShouldResemble, before)
			So(core.To[float64](regressed["count"]), ShouldEqual, core.To[float64](fields["count"]))
		})
	})
}

func BenchmarkPathNext(b *testing.B) {
	path := adaptive.NewPath(adaptive.NewWindow())
	var sequence int64
	b.ReportAllocs()

	for b.Loop() {
		sequence++
		// Repeated changes exercise shrinking and regrowing history.
		value := float64(100 + (sequence/64)%2*900 + sequence%3)

		if _, err := transport.Evaluate[map[string]core.Primitive](path, core.Record(map[string]any{"at": sequence, "value": value})); err != nil {
			b.Fatal(err)
		}
	}
}
