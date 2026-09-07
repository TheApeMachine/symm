package transport_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestFanNext(t *testing.T) {
	Convey("Given two identity branches sharing one capture", t, func() {
		fan := transport.NewFan(transport.NewPipe(), transport.NewIO(transport.NewPipe(), transport.NewPipe()))
		for _, values := range [][]float64{{1, 2, 3}, {}, {4}, {5, 6}} {
			output := tests.Drain(t, fan, tests.Values(values...))
			So(fan.Error(), ShouldBeNil)
			So(len(output), ShouldEqual, 2*len(values))

			for index, value := range output {
				So(value, ShouldEqual, values[index%len(values)])
			}
		}
	})
	Convey("Given no configured branches", t, func() {
		fan := transport.NewFan(transport.NewPipe(), nil)
		So(tests.Drain(t, fan, tests.Values(1.0, 2.0)), ShouldBeEmpty)
		So(fan.Error(), ShouldBeNil)
	})
}

func BenchmarkFanNext(b *testing.B) {
	fan := transport.NewFan(transport.NewPipe(), transport.NewIO(transport.NewPipe(), transport.NewPipe()))
	input := transport.NewIO(core.From(1.0), core.From(2.0), core.From(3.0))
	b.ReportAllocs()

	for b.Loop() {
		count := 0
		for output := fan.Next(input); output != nil; output = fan.Next(input) {
			count++
		}

		if count != 6 || fan.Error() != nil {
			b.Fatal("fan did not deliver both runs", fan.Error())
		}
	}
}
