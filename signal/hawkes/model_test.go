package hawkes_test

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	nmhawkes "github.com/theapemachine/symm/nomagique/statistic/hawkes"
	"github.com/theapemachine/symm/signal/hawkes"
)

var modelKeys = []string{
	"branching_spectral_radius",
}

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
		tape := mixedTape(origin, 180)

		Convey("It emits no model output before a model is fitted", func() {
			for step := range 10 {
				event := tape[step]
				res := instrument.Step(trade(event.at, int64(step+1), event.side, 50000, 1))
				So(res, ShouldNotBeNil)

				for _, key := range modelKeys {
					So(held(res, key), ShouldBeFalse)
				}
			}
		})

		Convey("It leaves the input frame's From alone and dates the output from the window origin", func() {
			for index := range nmhawkes.MaxArrivalSamples - 1 {
				event := tape[index]
				res := instrument.Step(trade(event.at, int64(index+1), event.side, 50000, 1))
				So(res, ShouldNotBeNil)
				So(res.From.Sub(origin).Abs(), ShouldBeLessThan, time.Microsecond)
			}
		})

		Convey("It keeps the model alive during a one-sided buy sweep", func() {
			ready := false
			var last *data.Measurement
			seq := int64(0)

			for _, event := range tape {
				seq++
				last = instrument.Step(trade(event.at, seq, event.side, 50000, 1))
				So(last, ShouldNotBeNil)
				ready = ready || held(last, "branching_spectral_radius")
			}

			So(ready, ShouldBeTrue)

			at := tape[len(tape)-1].at

			for range nmhawkes.MaxArrivalSamples {
				seq++
				at = at.Add(time.Duration(20+(int(seq)*37)%90) * time.Millisecond)
				last = instrument.Step(trade(at, seq, "buy", 50000, 1))
				So(last, ShouldNotBeNil)
			}

			for _, key := range modelKeys {
				So(held(last, key), ShouldBeTrue)
			}
		})
	})
}
