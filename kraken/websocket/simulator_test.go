package websocket

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

/*
FakeClock advances only when Sleep is called, for deterministic simulator tests.
*/
type FakeClock struct {
	now time.Time
}

/*
Now returns the fake clock's current time.
*/
func (clock *FakeClock) Now() time.Time {
	return clock.now
}

/*
Sleep advances the fake clock by wait without wall sleeping.
*/
func (clock *FakeClock) Sleep(_ context.Context, wait time.Duration) error {
	clock.now = clock.now.Add(wait)
	return nil
}

func TestSimulatorSeed(t *testing.T) {
	Convey("Given a newly constructed simulator", t, func() {
		simulator := NewSimulator()

		Convey("It should record a nonzero replay seed", func() {
			So(simulator.Seed(), ShouldNotEqual, 0)
			So(simulator.rng, ShouldNotBeNil)
		})
	})
}

func TestSimulatorPerStack(t *testing.T) {
	Convey("Given two latency simulators", t, func() {
		first := NewLatencySimulator(context.Background(), &FakeClock{now: time.Unix(1, 0)}, 7)
		second := NewLatencySimulator(context.Background(), &FakeClock{now: time.Unix(1, 0)}, 11)

		Convey("They are independent instances", func() {
			So(first, ShouldNotEqual, second)
			So(first.Seed(), ShouldEqual, 7)
			So(second.Seed(), ShouldEqual, 11)
		})
	})
}

func TestSimulatorRecordAndDo(t *testing.T) {
	Convey("Given a simulator with recorded websocket latency", t, func() {
		clock := &FakeClock{now: time.Unix(0, 0)}
		simulator := NewLatencySimulator(context.Background(), clock, 1)
		simulator.Record(WEBSOCKET, 25*time.Millisecond)

		before := clock.Now()
		simulator.Do(WEBSOCKET, func() {})

		Convey("It should advance the injected clock by the recorded latency", func() {
			So(clock.Now().Sub(before), ShouldEqual, 25*time.Millisecond)
		})
	})

	Convey("Given a fill latency request", t, func() {
		clock := &FakeClock{now: time.Unix(0, 0)}
		simulator := NewLatencySimulator(context.Background(), clock, 1)

		before := clock.Now()
		simulator.Do(FILL, func() {})

		Convey("It should advance by a fill sample from the ring", func() {
			elapsed := clock.Now().Sub(before)
			So(elapsed, ShouldBeGreaterThanOrEqualTo, 40*time.Millisecond)
			So(elapsed, ShouldBeLessThan, 500*time.Millisecond)
		})
	})
}

func BenchmarkSimulatorDo(b *testing.B) {
	simulator := NewLatencySimulator(
		context.Background(),
		&FakeClock{now: time.Unix(0, 0)},
		1,
	)

	for b.Loop() {
		simulator.Do(REST, func() {})
	}
}
