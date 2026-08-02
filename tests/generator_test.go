package tests

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGeneratorNewGenerator(t *testing.T) {
	Convey("Given a symbol name and start price", t, func() {
		generator := NewGenerator("SIM1/USD", 100.0, 42)

		Convey("It should initialize with default Baseline state", func() {
			So(generator, ShouldNotBeNil)
			So(generator.symbol, ShouldEqual, "SIM1/USD")
			So(generator.midPrice, ShouldEqual, 100.0)
			So(generator.currentState, ShouldEqual, Baseline)
		})
	})
}

func TestGeneratorSetState(t *testing.T) {
	Convey("Given a generator", t, func() {
		generator := NewGenerator("SIM1/USD", 100.0, 42)

		Convey("When SetState is called", func() {
			generator.SetState(FastPump)

			So(generator.targetState, ShouldEqual, FastPump)
		})
	})
}

func TestGeneratorStep(t *testing.T) {
	Convey("Given a generator", t, func() {
		generator := NewGenerator("SIM1/USD", 100.0, 42)

		Convey("When Step is called", func() {
			sample := generator.Step()

			So(sample.Symbol, ShouldEqual, "SIM1/USD")
			So(sample.Bid, ShouldBeGreaterThan, 0)
			So(sample.Ask, ShouldBeGreaterThan, sample.Bid)
			So(sample.Last, ShouldBeGreaterThan, 0)
			So(sample.Volume, ShouldBeGreaterThan, 0)
			So(sample.VWAP, ShouldBeGreaterThan, 0)
		})
	})
}

func TestGeneratorGenerate(t *testing.T) {
	Convey("Given a generator and JSON template", t, func() {
		generator := NewGenerator("SIM1/USD", 100.0, 42)
		template := []byte(`{"channel":"ticker","type":"snapshot","data":[{"symbol":"SIM1/USD","bid":100.0}]}`)

		Convey("When Generate is called", func() {
			count := 0

			for frame := range generator.Generate(template) {
				So(len(frame), ShouldBeGreaterThan, 0)
				count++
			}

			So(count, ShouldEqual, 1)
		})
	})
}

func BenchmarkGeneratorStep(b *testing.B) {
	generator := NewGenerator("SIM1/USD", 100.0, 42)

	for b.Loop() {
		_ = generator.Step()
	}
}
