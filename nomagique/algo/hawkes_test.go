package algo

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	nomahawkes "github.com/theapemachine/nomagique/hawkes"
	"github.com/theapemachine/symm/nomagique/types"
)

func TestNewHawkes(t *testing.T) {
	Convey("Given a new hawkes algorithm", t, func() {
		process := NewHawkes()

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
		process := NewHawkes()
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		process.Write(stageTrade("ALT/EUR", "buy", base))

		Convey("Write should stage without measuring", func() {
			So(process.Error(), ShouldBeBlank)
			So(process.Project().Ready, ShouldBeFalse)
		})
	})

	Convey("Given an arrival without a timestamp", t, func() {
		process := NewHawkes()
		collected := types.NewMap[string, types.Value[float64]]()
		collected.Put("side", types.NewValue(1.0))
		process.Write(stagePair("ALT/EUR", collected))

		Convey("Write should report a validation error", func() {
			So(process.Error(), ShouldNotBeBlank)
		})
	})
}

func TestRead(t *testing.T) {
	Convey("Given one event without an identifiable interval", t, func() {
		process := NewHawkes()
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		process.Write(stageTrade("ALT/EUR", "buy", base))
		process.Read()

		Convey("It should publish only the count observation", func() {
			So(process.Error(), ShouldBeBlank)
			So(lookup(process, "ready"), ShouldEqual, 1)
			So(lookup(process, "observation"), ShouldEqual, 1)
			So(lookup(process, "intensity"), ShouldEqual, 0)
			So(lookup(process, "event_count"), ShouldEqual, 1)
		})
	})

	Convey("Given a stream without a symbol", t, func() {
		process := NewHawkes()
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		process.Write(stageTrade("", "buy", base))

		Convey("Write should return a validation error", func() {
			So(process.Error(), ShouldNotBeBlank)
		})
	})

	Convey("Given enough typed arrivals to identify the exponential model", t, func() {
		process := NewHawkes()
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		writeBurst(process, base, 32)
		process.Read()

		Convey("It should publish fitted state without promoting it to a forecast", func() {
			So(process.Error(), ShouldBeBlank)
			So(lookup(process, "ready"), ShouldEqual, 1)
			So(lookup(process, "intensity"), ShouldEqual, 1)
			So(lookup(process, "hawkes_fit"), ShouldEqual, 1)
			So(lookup(process, "model_updated"), ShouldEqual, 1)
			So(lookup(process, "forecast"), ShouldEqual, 0)
			So(lookup(process, "fit_valid"), ShouldEqual, 1)
			So(lookup(process, "maturity"), ShouldEqual, 1)
		})

		Convey("When the unchanged stream is measured again", func() {
			fitAt := lookup(process, "fit_at_nsec")
			process.Read()

			Convey("It should reuse the fit only for current conditional intensity", func() {
				So(process.Error(), ShouldBeBlank)
				So(lookup(process, "hawkes_fit"), ShouldEqual, 1)
				So(lookup(process, "model_updated"), ShouldEqual, 0)
				So(lookup(process, "fit_at_nsec"), ShouldEqual, fitAt)
			})
		})

		Convey("It should compare full, Poisson, and re-estimated self-only likelihoods", func() {
			stream, horizon := burstStream(base, 32)
			context, contextReady := nomahawkes.NewObservationContext(stream, horizon)
			So(contextReady, ShouldBeTrue)
			poisson := context.PoissonFit().WithIntensitiesAt(stream, horizon)
			selfOnly := nomahawkes.NewBivariateEstimator(nomahawkes.BivariateFit{}).
				FitSelfOnly(stream, horizon)
			full := nomahawkes.NewBivariateEstimator(nomahawkes.BivariateFit{}).
				Fit(stream, horizon)
			So(lookup(process, "ll_delta_poisson"), ShouldAlmostEqual,
				full.LogLikelihood(stream, horizon)-poisson.LogLikelihood(stream, horizon), 1e-9)
			So(lookup(process, "ll_delta_self"), ShouldAlmostEqual,
				full.LogLikelihood(stream, horizon)-selfOnly.LogLikelihood(stream, horizon), 1e-9)
		})
	})

	Convey("Given a dense one-sided stream", t, func() {
		process := NewHawkes()
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

		for index := range 32 {
			process.Write(stageTrade("ALT/EUR", "buy", base.Add(time.Duration(index)*time.Millisecond)))
		}

		process.Read()

		Convey("It should retain empirical rates without inventing the empty side", func() {
			So(process.Error(), ShouldBeBlank)
			So(lookup(process, "intensity"), ShouldEqual, 1)
			So(lookup(process, "hawkes_fit"), ShouldEqual, 0)
			So(lookup(process, "alpha_arrival_rate"), ShouldBeGreaterThan, 0)
			So(lookup(process, "beta_arrival_rate"), ShouldEqual, 0)
			So(lookup(process, "maturity"), ShouldEqual, 0)
			So(lookup(process, "mu_buy"), ShouldEqual, lookup(process, "alpha_arrival_rate"))
			So(lookup(process, "mu_sell"), ShouldEqual, 0)
			So(lookup(process, "lambda_buy"), ShouldEqual, lookup(process, "alpha_arrival_rate"))
			So(process.Reason(), ShouldContainSubstring, "per side")
		})
	})

	Convey("Given side events on a common explicit observation interval", t, func() {
		process := NewHawkes()
		origin := time.Date(2026, 5, 30, 13, 0, 0, 0, time.UTC)
		horizon := origin.Add(4 * time.Second)
		process.Write(stageTrade("ALT/EUR", "buy", origin))
		process.Write(stageTrade("ALT/EUR", "buy", origin.Add(time.Second)))
		process.Write(stageTrade("ALT/EUR", "sell", origin.Add(2*time.Second)))
		process.Write(stageTrade("ALT/EUR", "sell", origin.Add(3*time.Second)))
		process.Write(stageTrade("ALT/EUR", "buy", horizon))
		process.Read()

		Convey("It should publish exact (S,T] counts and rates with the same denominator", func() {
			So(process.Error(), ShouldBeBlank)
			So(lookup(process, "observed_from_sec"), ShouldEqual, float64(origin.Unix()))
			So(lookup(process, "observed_at_sec"), ShouldEqual, float64(horizon.Unix()))
			So(lookup(process, "alpha_event_count"), ShouldEqual, 2)
			So(lookup(process, "beta_event_count"), ShouldEqual, 2)
			So(lookup(process, "alpha_arrival_rate"), ShouldEqual, 0.5)
			So(lookup(process, "beta_arrival_rate"), ShouldEqual, 0.5)
		})
	})

	Convey("Given distinct trades one nanosecond apart", t, func() {
		process := NewHawkes()
		base := time.Date(2026, 5, 30, 12, 0, 0, 1, time.UTC)
		process.Write(stageTrade("ALT/EUR", "buy", base))
		process.Write(stageTrade("ALT/EUR", "sell", base.Add(time.Nanosecond)))
		process.Write(stageTrade("ALT/EUR", "buy", base.Add(2*time.Nanosecond)))
		process.Read()

		Convey("It should preserve native timestamps exactly", func() {
			So(process.Error(), ShouldBeBlank)
			So(lookup(process, "observed_from_sec"), ShouldEqual, float64(base.Unix()))
			So(lookup(process, "observed_from_nsec"), ShouldEqual, float64(base.Nanosecond()))
		})
	})
}

func TestProject(t *testing.T) {
	Convey("Given a hawkes algorithm after Read", t, func() {
		process := NewHawkes()
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		process.Write(stageTrade("ALT/EUR", "buy", base))
		process.Read()

		Convey("Project should expose the measured pair", func() {
			So(process.Project().Read().Key, ShouldEqual, "ALT/EUR")
			So(lookup(process, "event_count"), ShouldEqual, 1)
		})
	})
}

func TestError(t *testing.T) {
	Convey("Given a hawkes algorithm with no arrivals", t, func() {
		process := NewHawkes()
		process.Read()

		Convey("Read should report a missing stream", func() {
			So(process.Error(), ShouldNotBeBlank)
		})
	})
}

func TestClose(t *testing.T) {
	Convey("Given a hawkes algorithm", t, func() {
		process := NewHawkes()

		Convey("Close should succeed", func() {
			So(process.Close(), ShouldBeNil)
		})
	})
}

func TestAwaitFit(t *testing.T) {
	Convey("Given a fitted sixteen-event epoch", t, func() {
		process := NewHawkes()
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		writeBurst(process, base, 16)
		process.Read()
		So(lookup(process, "model_updated"), ShouldEqual, 1)

		Convey("It should refit asynchronously at the square-root sampling-error scale", func() {
			for added := 1; added <= 4; added++ {
				side := "sell"

				if (16+added-1)%2 != 0 {
					side = "buy"
				}

				process.Write(stageTrade(
					"ALT/EUR",
					side,
					base.Add(time.Duration(16+added-1)*100*time.Millisecond),
				))
				process.Read()
				So(process.Error(), ShouldBeBlank)

				if added < 4 {
					So(lookup(process, "model_updated"), ShouldEqual, 0)
				}
			}

			So(lookup(process, "model_updated"), ShouldEqual, 0)
			So(process.AwaitFit(), ShouldBeTrue)
			process.Read()
			So(lookup(process, "model_updated"), ShouldEqual, 1)
		})
	})
}

func TestReason(t *testing.T) {
	Convey("Given a one-sided stream", t, func() {
		process := NewHawkes()
		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)

		for index := range 32 {
			process.Write(stageTrade("ALT/EUR", "buy", base.Add(time.Duration(index)*time.Millisecond)))
		}

		process.Read()

		Convey("Reason should explain the missing side", func() {
			So(process.Reason(), ShouldContainSubstring, "per side")
		})
	})
}

func lookup(process *Hawkes, name string) float64 {
	value, found := process.Project().Read().Value.Get(name)

	if !found {
		return 0
	}

	return value.Read()
}

func stageTrade(symbol string, side string, at time.Time) types.Input[hawkesState] {
	mark := 1.0

	if side == "sell" {
		mark = -1
	}

	collected := types.NewMap[string, types.Value[float64]]()
	collected.Put("side", types.NewValue(mark))
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
	for index := range count {
		side := "sell"

		if index%2 != 0 {
			side = "buy"
		}

		process.Write(stageTrade("ALT/EUR", side, base.Add(time.Duration(index)*100*time.Millisecond)))
	}
}

func burstStream(base time.Time, count int) (nomahawkes.ArrivalStream, time.Time) {
	buyTimes := make([]time.Time, 0, count/2)
	sellTimes := make([]time.Time, 0, count/2)

	for index := range count {
		eventTime := base.Add(time.Duration(index) * 100 * time.Millisecond)

		if index%2 == 0 {
			sellTimes = append(sellTimes, eventTime)

			continue
		}

		buyTimes = append(buyTimes, eventTime)
	}

	return nomahawkes.NewArrivalStream(buyTimes, sellTimes),
		base.Add(time.Duration(count) * 100 * time.Millisecond)
}
