package transport_test

import (
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestPipeAndMap(t *testing.T) {
	pipe := transport.NewPipe(
		transport.NewMap(calculus.NewSquare(transport.NewIO(core.From(0.0)))),
		arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))),
	)
	for range 3 {
		tests.EqualNumber(t, tests.Drain(t, pipe, transport.NewIO(core.From(2.0), core.From(3.0)))[0], 13)
	}
}
func TestFanIsOpaqueTransport(t *testing.T) {
	fan := transport.NewFan(transport.NewPipe(), transport.NewIO(
		arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0))),
		transport.NewMapReduce(calculus.NewSquare(transport.NewIO(core.From(0.0))), arithmetic.NewAdd[float64](transport.NewIO(core.From(0.0)))),
	))
	got := tests.Drain(t, fan, transport.NewIO(core.From(2.0), core.From(3.0)))
	if len(got) != 2 {
		t.Fatal(got)
	}
	tests.EqualNumber(t, got[0], 5)
	tests.EqualNumber(t, got[1], 13)
}

func TestPipeNext(t *testing.T) {
	Convey("Given a pipeline that reuses the same stage three times", t, func() {
		stage := transport.NewMap(transport.NewPipe())
		pipe := transport.NewPipe(stage, stage, stage)
		for _, values := range [][]float64{{1, 2, 3}, {}, {4}, {5, 6}} {
			output := tests.Drain(t, pipe, tests.Values(values...))
			So(pipe.Error(), ShouldBeNil)
			So(len(output), ShouldEqual, len(values))

			for index, value := range output {
				So(value, ShouldEqual, values[index])
			}
		}
	})
}

func BenchmarkPipeNext(b *testing.B) {
	stage := transport.NewPipe()
	pipe := transport.NewPipe(stage, stage, stage)
	input := transport.NewIO(core.From(1.0), core.From(2.0), core.From(3.0))
	b.ReportAllocs()

	for b.Loop() {
		count := 0
		for output := pipe.Next(input); output != nil; output = pipe.Next(input) {
			count++
		}

		if count != 3 || pipe.Error() != nil {
			b.Fatal("pipe did not deliver the run", pipe.Error())
		}
	}
}
