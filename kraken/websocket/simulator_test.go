package websocket

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSimulatorSeed(t *testing.T) {
	Convey("Given a newly constructed simulator", t, func() {
		simulator := NewSimulator()

		Convey("It should record a nonzero replay seed", func() {
			So(simulator.Seed(), ShouldNotEqual, 0)
			So(simulator.rng, ShouldNotBeNil)
		})
	})
}

func TestSimulatorRecordAndDo(t *testing.T) {
	Convey("Given a simulator with recorded websocket latency", t, func() {
		simulator := NewSimulator()
		simulator.Initialize()
		simulator.Record(WEBSOCKET, 25*time.Millisecond)

		started := time.Now()

		simulator.Do(WEBSOCKET, func() {})

		elapsed := time.Since(started)

		Convey("It should replay a recorded websocket latency", func() {
			So(elapsed, ShouldBeGreaterThanOrEqualTo, 20*time.Millisecond)
		})
	})

	Convey("Given a fill latency request", t, func() {
		simulator := NewSimulator()
		simulator.Initialize()

		started := time.Now()

		simulator.Do(FILL, func() {})

		elapsed := time.Since(started)

		Convey("It should use a realistic random fill delay", func() {
			So(elapsed, ShouldBeGreaterThanOrEqualTo, 35*time.Millisecond)
			So(elapsed, ShouldBeLessThan, 500*time.Millisecond)
		})
	})
}

func BenchmarkSimulatorDo(b *testing.B) {
	simulator := NewSimulator()
	simulator.Initialize()

	for b.Loop() {
		simulator.Do(REST, func() {})
	}
}
