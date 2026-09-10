package adaptive_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/adaptive"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestPathNext(t *testing.T) {
	Convey("Given a path with observation-driven support retention", t, func() {
		window := adaptive.NewWindow()
		path := adaptive.NewPath(window)
		expected := adaptive.NewWindow()
		retained := []float64{}
		var reading equation.Price
		var firstCount float64
		shed := false
		var at int64

		for phase, level := range []float64{100, 1000, 10, 500} {
			for index := range 64 {
				at++
				value := level + float64(index%3)
				policy := expected.Observe(value)
				retained = append(retained, value)

				if len(retained) > int(policy.Capacity) {
					retained = retained[len(retained)-int(policy.Capacity):]
					shed = true
				}

				out, err := transport.Evaluate(path, transport.Values(equation.Price{At: at, Value: value}))
				So(err, ShouldBeNil)
				So(len(out.Observations), ShouldEqual, len(retained))

				for offset, observation := range out.Observations {
					So(observation.Value, ShouldEqual, retained[offset])
				}

				if phase == 0 && index == 0 {
					firstCount = out.Count
				}

				reading = equation.Price{At: at, Value: value}
			}
		}

		So(shed, ShouldBeTrue)
		So(firstCount, ShouldEqual, 1)
		_ = reading

		Convey("A regressed event does not advance the retention policy or edit history", func() {
			before := window.Reading
			regressed, err := transport.Evaluate(path, transport.Values(equation.Price{At: at - 1, Value: -1000}))
			So(err, ShouldBeNil)
			So(regressed.Accepted, ShouldBeFalse)
			So(window.Reading, ShouldResemble, before)
			So(regressed.Count, ShouldEqual, float64(len(retained)))
		})
	})
}

func BenchmarkPathNext(b *testing.B) {
	path := adaptive.NewPath(adaptive.NewWindow())
	var sequence int64
	b.ReportAllocs()

	for b.Loop() {
		sequence++
		value := float64(100 + (sequence/64)%2*900 + sequence%3)

		if _, err := transport.Evaluate(path, transport.Values(equation.Price{At: sequence, Value: value})); err != nil {
			b.Fatal(err)
		}
	}
}
