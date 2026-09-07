package transport_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestMapNext(t *testing.T) {
	Convey("Given a mapper that emits each input twice", t, func() {
		mapping := transport.NewMap(transport.NewFan(transport.NewPipe(), transport.NewIO(transport.NewPipe(), transport.NewPipe())))
		for _, values := range [][]float64{{1, 2, 3}, {}, {4}, {5, 6}} {
			output := tests.Drain(t, mapping, tests.Values(values...))
			So(mapping.Error(), ShouldBeNil)
			So(len(output), ShouldEqual, 2*len(values))

			for index, value := range output {
				So(value, ShouldEqual, values[index/2])
			}
		}
	})
}

func BenchmarkMapNext(b *testing.B) {
	mapping := transport.NewMap(transport.NewPipe())
	input := transport.NewIO(core.From(1.0), core.From(2.0), core.From(3.0))
	b.ReportAllocs()

	for b.Loop() {
		count := 0
		for output := mapping.Next(input); output != nil; output = mapping.Next(input) {
			count++
		}

		if count != 3 || mapping.Error() != nil {
			b.Fatal("map did not deliver the run", mapping.Error())
		}
	}
}
