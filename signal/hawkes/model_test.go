package hawkes_test

import (
	"context"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	nmhawkes "github.com/theapemachine/symm/nomagique/statistic/hawkes"
	"github.com/theapemachine/symm/signal/hawkes"
)

/*
modelKeys are outputs that only exist once a Hawkes model has been fitted.
*/
var modelKeys = []string{
	"conditional_intensity",
	"background_rate",
	"excitation_decay",
	"branching_spectral_radius",
	"log_likelihood:hawkes",
	"log_likelihood:poisson",
	"log_likelihood_gain_vs_poisson",
	"compensator:buy",
	"count_innovation:buy",
}

/*
tapeEvent is one trade of a deterministic tape with irregular gaps, so the
gap quartiles differ and the fit context is identifiable.
*/
type tapeEvent struct {
	at   time.Time
	side string
}

func mixedTape(origin time.Time, count int) []tapeEvent {
	tape := make([]tapeEvent, 0, count)
	at := origin

	for step := range count {
		side := "buy"

		if (step+1)%3 == 0 {
			side = "sell"
		}

		tape = append(tape, tapeEvent{at: at, side: side})
		at = at.Add(time.Duration(20+(step*37)%90) * time.Millisecond)
	}

	return tape
}

func held(measurement *data.Measurement, label string) bool {
	_, ok := metric(measurement, label)
	return ok
}

func TestHawkesModelOutputs(t *testing.T) {
	Convey("Given a READY Hawkes signal fed one symbol's trade tape", t, func() {
		instrument := hawkes.NewSignal(context.Background())
		instrument.Transition(nmruntime.READY)
		origin := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		tape := mixedTape(origin, 60)

		Convey("It emits no model output before a model is fitted", func() {
			res := instrument.Step(trade(tape[0].at, 1, tape[0].side, 50000, 1))
			So(res, ShouldNotBeNil)
			So(held(res, "event_count"), ShouldBeTrue)

			for _, key := range modelKeys {
				So(held(res, key), ShouldBeFalse)
			}
		})

		Convey("It leaves the input frame's From alone and dates the output from the window origin", func() {
			for index, event := range tape {
				prior := trade(event.at, int64(index+1), event.side, 50000, 1)
				res := instrument.Step(prior)
				So(res, ShouldNotBeNil)
				So(prior.From, ShouldEqual, event.at)
				So(res.From, ShouldEqual, tape[max(0, index-nmhawkes.MaxArrivalSamples)].at)
			}
		})

		Convey("It scores Poisson as the window's homogeneous MLE and counts innovation on the compensator's interval", func() {
			scored := 0

			for index, event := range tape {
				res := instrument.Step(trade(event.at, int64(index+1), event.side, 50000, 1))
				So(res, ShouldNotBeNil)

				poissonLL, ok := metric(res, "log_likelihood:poisson")

				if !ok {
					continue
				}

				scored++

				// The window is every event so far (fewer than the retained
				// capacity); arrivals at the origin are prehistory, so the
				// counted events are indices 1..index.
				var buys, sells float64

				for _, counted := range tape[1 : index+1] {
					if counted.side == "buy" {
						buys++
						continue
					}

					sells++
				}

				// The signal works on float epoch seconds (~2e-7 s resolution
				// at 2026 epochs), hence the tolerance below.
				span := event.at.Sub(tape[0].at).Seconds()
				So(span, ShouldBeGreaterThan, 0)
				So(buys, ShouldBeGreaterThan, 0)
				So(sells, ShouldBeGreaterThan, 0)
				expected := buys*math.Log(buys/span) - buys + sells*math.Log(sells/span) - sells
				So(poissonLL, ShouldAlmostEqual, expected, 1e-4)

				hawkesLL := metricValue(res, "log_likelihood:hawkes")
				So(metricValue(res, "log_likelihood_gain_vs_poisson"), ShouldAlmostEqual, hawkesLL-poissonLL, 1e-9)

				// Innovation is evaluated on the prior events over
				// (origin, at): the origin arrival is not an observation.
				priorBuys := buys
				priorSells := sells

				if event.side == "buy" {
					priorBuys--
				} else {
					priorSells--
				}

				So(
					metricValue(res, "count_innovation:buy")+metricValue(res, "compensator:buy"),
					ShouldAlmostEqual, priorBuys, 1e-9,
				)
				So(
					metricValue(res, "count_innovation:sell")+metricValue(res, "compensator:sell"),
					ShouldAlmostEqual, priorSells, 1e-9,
				)
			}

			So(scored, ShouldBeGreaterThan, 0)
		})

		Convey("It drops the model once the window can no longer identify one", func() {
			ready := false
			var last *data.Measurement
			seq := int64(0)

			for _, event := range tape {
				seq++
				last = instrument.Step(trade(event.at, seq, event.side, 50000, 1))
				So(last, ShouldNotBeNil)
				ready = ready || held(last, "conditional_intensity")
			}

			So(ready, ShouldBeTrue)

			at := tape[len(tape)-1].at

			for range nmhawkes.MaxArrivalSamples {
				seq++
				at = at.Add(time.Duration(20+(int(seq)*37)%90) * time.Millisecond)
				last = instrument.Step(trade(at, seq, "buy", 50000, 1))
				So(last, ShouldNotBeNil)
			}

			So(metricValue(last, "event_count:sell"), ShouldBeLessThan, metricValue(last, "event_count")/4)

			for _, key := range modelKeys {
				So(held(last, key), ShouldBeFalse)
			}
		})
	})
}
