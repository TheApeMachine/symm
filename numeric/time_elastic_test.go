package numeric

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTimeElasticMemoryUpdate(t *testing.T) {
	halflife := time.Hour
	start := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)

	Convey("Given a cold start", t, func() {
		memory := NewTimeElasticMemory(halflife, 0)

		relative, err := memory.Update(start, 10)

		Convey("It should seed neutral RVOL", func() {
			So(err, ShouldBeNil)
			So(relative, ShouldEqual, 1.0)
			So(memory.Initialized(), ShouldBeTrue)
		})
	})

	Convey("Given a burst after a dense baseline", t, func() {
		memory := NewTimeElasticMemory(halflife, 0)

		_, err := memory.Update(start, 1)
		So(err, ShouldBeNil)

		relative, err := memory.Update(
			start.Add(200*time.Millisecond),
			20,
		)

		Convey("It should exceed unity", func() {
			So(err, ShouldBeNil)
			So(relative, ShouldBeGreaterThan, 1.0)
		})
	})

	Convey("Given a long silence on a thin pair", t, func() {
		memory := NewTimeElasticMemory(time.Minute, 1e-6)

		_, err := memory.Update(start, 1000)
		So(err, ShouldBeNil)

		_, err = memory.Update(start.Add(time.Second), 1000)
		So(err, ShouldBeNil)

		relative, err := memory.Update(
			start.Add(10*time.Minute),
			5,
		)

		Convey("It should not phantom-spike stale volume context", func() {
			So(err, ShouldBeNil)
			So(relative, ShouldBeLessThan, 0.1)
		})
	})

	Convey("Given a negative sample", t, func() {
		memory := NewTimeElasticMemory(halflife, 0)

		_, err := memory.Update(start, -1)

		Convey("It should return an error", func() {
			So(err, ShouldNotBeNil)
		})
	})
}

func BenchmarkTimeElasticMemoryUpdate(b *testing.B) {
	memory := NewTimeElasticMemory(time.Hour, 1e-6)
	at := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)

	b.ReportAllocs()

	for b.Loop() {
		_, err := memory.Update(at, 10)

		if err != nil {
			b.Fatal(err)
		}

		at = at.Add(time.Millisecond)
	}
}
