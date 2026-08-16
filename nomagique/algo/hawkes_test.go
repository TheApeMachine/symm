package algo

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestNewHawkes(t *testing.T) {
	Convey("Given a new hawkes algorithm", t, func() {
		process := NewHawkes(types.NewInput[hawkesState]())

		Convey("It should implement IO", func() {
			var input types.Input[hawkesState] = process
			var output types.Output[hawkesState] = process
			So(input, ShouldNotBeNil)
			So(output, ShouldNotBeNil)
			So(process.Error(), ShouldBeBlank)
		})
	})
}

func TestWrite(t *testing.T) {
	Convey("Given a hawkes algorithm and one arrival", t, func() {
		process := NewHawkes(types.NewInput[hawkesState]())
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		process.Write(stageArrival("STREAM/A", "alpha", base))

		Convey("Write should stage without error", func() {
			So(process.Error(), ShouldBeBlank)
		})
	})

	Convey("Given an arrival without a timestamp", t, func() {
		process := NewHawkes(types.NewInput[hawkesState]())
		collected := types.NewMap[string, types.Value[float64]]()
		collected.Put("mark", types.NewValue(1.0))
		process.Write(stagePair("STREAM/A", collected))
		process.Read()

		Convey("Read should report a validation error", func() {
			So(process.Error(), ShouldNotBeBlank)
		})
	})
}

func TestRead(t *testing.T) {
	Convey("Given one event without an identifiable interval", t, func() {
		process := NewHawkes(types.NewInput[hawkesState]())
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		process.Write(stageArrival("STREAM/A", "alpha", base))
		process.Read()

		Convey("It should publish the count observation", func() {
			So(process.Error(), ShouldBeBlank)
			So(lookup(process, "ready"), ShouldEqual, 1)
			So(lookup(process, "observation"), ShouldEqual, 1)
			So(lookup(process, "event_count"), ShouldEqual, 1)
			So(lookup(process, "alpha_event_count"), ShouldEqual, 1)
			So(lookup(process, "beta_event_count"), ShouldEqual, 0)
		})
	})

	Convey("Given a stream without a symbol", t, func() {
		process := NewHawkes(types.NewInput[hawkesState]())
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		process.Write(stageArrival("", "alpha", base))
		process.Read()

		Convey("Read should return a validation error", func() {
			So(process.Error(), ShouldNotBeBlank)
		})
	})

	Convey("Given typed arrivals over time", t, func() {
		process := NewHawkes(types.NewInput[hawkesState]())
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		writeBurst(process, base, 32)

		Convey("It should publish measured Hawkes state", func() {
			So(process.Error(), ShouldBeBlank)
			So(lookup(process, "ready"), ShouldEqual, 1)
			So(lookup(process, "event_count"), ShouldEqual, 32)
			So(lookup(process, "alpha_event_count"), ShouldEqual, 16)
			So(lookup(process, "beta_event_count"), ShouldEqual, 16)
			So(lookup(process, "spectral_radius"), ShouldBeGreaterThan, 0)
			So(lookup(process, "lambda_alpha"), ShouldBeGreaterThan, 0)
			So(lookup(process, "lambda_beta"), ShouldBeGreaterThan, 0)
		})
	})

	Convey("Given arrival events on an observation interval", t, func() {
		process := NewHawkes(types.NewInput[hawkesState]())
		origin := time.Date(2026, 5, 30, 13, 0, 0, 0, time.UTC)
		horizon := origin.Add(4 * time.Second)

		process.Write(stageArrival("STREAM/A", "alpha", origin))
		process.Read()

		process.Write(stageArrival("STREAM/A", "alpha", origin.Add(time.Second)))
		process.Read()

		process.Write(stageArrival("STREAM/A", "beta", origin.Add(2*time.Second)))
		process.Read()

		process.Write(stageArrival("STREAM/A", "beta", origin.Add(3*time.Second)))
		process.Read()

		process.Write(stageArrival("STREAM/A", "alpha", horizon))
		process.Read()

		Convey("It should publish exact timestamps and arrival counts", func() {
			So(process.Error(), ShouldBeBlank)
			So(lookup(process, "observed_from_sec"), ShouldEqual, float64(origin.Unix()))
			So(lookup(process, "observed_at_sec"), ShouldEqual, float64(horizon.Unix()))
			So(lookup(process, "alpha_event_count"), ShouldEqual, 3)
			So(lookup(process, "beta_event_count"), ShouldEqual, 2)
		})
	})
}

func TestProject(t *testing.T) {
	Convey("Given a hawkes algorithm after Read", t, func() {
		process := NewHawkes(types.NewInput[hawkesState]())
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		process.Write(stageArrival("STREAM/A", "alpha", base))
		process.Read()

		Convey("Project should expose the measured pair", func() {
			So(process.Project().Read().Key, ShouldEqual, "STREAM/A")
			So(lookup(process, "event_count"), ShouldEqual, 1)
		})
	})
}

func TestClose(t *testing.T) {
	Convey("Given a hawkes algorithm", t, func() {
		process := NewHawkes(types.NewInput[hawkesState]())

		Convey("Close should succeed", func() {
			So(process.Close(), ShouldBeNil)
		})
	})
}

func BenchmarkHawkes(b *testing.B) {
	process := NewHawkes(types.NewInput[hawkesState]())
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	arrival := stageArrival("STREAM/A", "alpha", base)

	b.ResetTimer()

	for b.Loop() {
		process.Write(arrival)
		_ = process.Read()
	}
}

func lookup(process *Hawkes, name string) float64 {
	val, found := process.Project().Read().Value.Get(name)

	if !found {
		return 0
	}

	return val.Read()
}

func stageArrival(symbol string, mark string, at time.Time) types.Input[hawkesState] {
	sign := 1.0

	if mark == "beta" {
		sign = -1.0
	}

	collected := types.NewMap[string, types.Value[float64]]()
	collected.Put("mark", types.NewValue(sign))
	collected.Put("unix_sec", types.NewValue(float64(at.Unix())))
	collected.Put("unix_nsec", types.NewValue(float64(at.Nanosecond())))

	return stagePair(symbol, collected)
}

func stagePair(
	symbol string,
	collected types.Map[string, types.Value[float64]],
) types.Input[hawkesState] {
	return &types.InputValue[hawkesState]{
		Value: types.NewValue(types.NewPair(symbol, collected)),
	}
}

func writeBurst(process *Hawkes, base time.Time, count int) {
	state := types.NewMap[string, types.Value[float64]]()

	for index := range count {
		sign := -1.0

		if index%2 != 0 {
			sign = 1.0
		}

		eventTime := base.Add(time.Duration(index) * 100 * time.Millisecond)
		state.Put("mark", types.NewValue(sign))
		state.Put("unix_sec", types.NewValue(float64(eventTime.Unix())))
		state.Put("unix_nsec", types.NewValue(float64(eventTime.Nanosecond())))

		process.Write(stagePair("STREAM/A", state))
		process.Read()
	}
}
