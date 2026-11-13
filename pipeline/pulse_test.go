package pipeline

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDefine(t *testing.T) {
	Convey("Setup", t, func() {
		Convey("When Define registers trade under kraken", func() {
			pulse := Define("pulse-trade-test", "kraken")

			Convey("the pulse id and parent are stored", func() {
				So(pulse.id, ShouldEqual, "pulse-trade-test")
				So(pulse.parent, ShouldEqual, "kraken")
			})
		})

		Convey("When Define is called with an empty id", func() {
			Convey("it returns nil", func() {
				So(Define("", "kraken"), ShouldBeNil)
			})
		})
	})
}

func TestHit(t *testing.T) {
	Convey("Setup", t, func() {
		id := "pulse-hit-test"
		Define(id, "kraken")
		before := Define(id, "").n.Load()

		Convey("When Hit marks activity", func() {
			Hit(id)
			row := Define(id, "").snapshot()

			Convey("n increments by one", func() {
				So(row["id"], ShouldEqual, id)
				So(row["parent"], ShouldEqual, "kraken")
				So(row["n"], ShouldEqual, before+1)
			})
		})
	})
}

func TestTime(t *testing.T) {
	Convey("Setup", t, func() {
		id := "pulse-time-test"
		Define(id, "training")

		Convey("When Time records 1ms", func() {
			Time(id, time.Millisecond)
			row := Define(id, "").snapshot()

			Convey("ema_ns equals the sample", func() {
				So(row["ema_ns"], ShouldEqual, int64(time.Millisecond))
				So(row["last_ns"], ShouldEqual, int64(time.Millisecond))
			})
		})
	})
}

func TestQueue(t *testing.T) {
	Convey("Setup", t, func() {
		id := "pulse-queue-test"
		Define(id, "store")

		Convey("When Queue sets depth 42", func() {
			Queue(id, 42)
			row := Define(id, "").snapshot()

			Convey("the snapshot carries queue depth", func() {
				So(row["queue"], ShouldEqual, int64(42))
			})
		})
	})
}

func TestWrote(t *testing.T) {
	Convey("Setup", t, func() {
		id := "pulse-wrote-queue"
		Define("queue", "store")
		Define(id, "store")

		Convey("When Wrote reports depth 7", func() {
			Wrote(7)
			row := Define("queue", "").snapshot()

			Convey("queue depth and activity update", func() {
				So(row["queue"], ShouldEqual, int64(7))
			})
		})
	})
}

func TestFire(t *testing.T) {
	Convey("Setup", t, func() {
		id := "pulse-fire-test"
		Define(id, "")

		Convey("When Fire publishes on a defined pulse", func() {
			Fire(id, map[string]any{"ema_ns": int64(9000)})

			Convey("the pulse stays registered", func() {
				So(Define(id, "").id, ShouldEqual, id)
			})
		})
	})
}

func BenchmarkDefine(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_ = Define("bench-pulse-define", "kraken")
	}
}

func BenchmarkHit(b *testing.B) {
	id := "bench-pulse-hit"
	Define(id, "kraken")

	b.ReportAllocs()

	for b.Loop() {
		Hit(id)
	}
}

func BenchmarkTime(b *testing.B) {
	id := "bench-pulse-time"
	Define(id, "training")

	b.ReportAllocs()

	for b.Loop() {
		Time(id, time.Millisecond)
	}
}

func BenchmarkQueue(b *testing.B) {
	id := "bench-pulse-queue"
	Define(id, "store")

	b.ReportAllocs()

	for b.Loop() {
		Queue(id, 42)
	}
}

func BenchmarkWrote(b *testing.B) {
	Define("queue", "store")
	Define("bench-pulse-wrote", "store")

	b.ReportAllocs()

	for b.Loop() {
		Wrote(7)
	}
}

func BenchmarkFire(b *testing.B) {
	id := "bench-pulse-fire"
	Define(id, "")
	payload := map[string]any{"ema_ns": int64(9000)}

	b.ReportAllocs()

	for b.Loop() {
		Fire(id, payload)
	}
}
