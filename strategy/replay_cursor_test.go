package strategy

import (
	"container/ring"
	"context"
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative/reward"
	"github.com/theapemachine/symm/nomagique/runtime"
)

func TestReplayCursorStep(t *testing.T) {
	Convey("A worker advances one observation per workload step", t, func() {
		rehearsal := rehearsalFixture(t)
		episodes := rehearsalEpisodes(rehearsal, 6)
		first := mustFragment(t, rehearsal, episodes[0])
		second := mustFragment(t, rehearsal, episodes[1])
		cursor := rehearsal.cursors[0]
		cursor.pending = []fragment{first}
		learned := cursor.learned
		cursor.Step(nil)
		So(rehearsal.Error(), ShouldBeNil)
		So(cursor.index, ShouldEqual, 1)
		start := cursor.fragments
		cursor.pending = []fragment{second}

		for step := 0; step < 2*len(first.observations)+2 && rehearsal.Wire().Passes < 1; step++ {
			cursor.Step(nil)
		}
		So(rehearsal.Error(), ShouldBeNil)
		So(rehearsal.Wire().Passes, ShouldEqual, 1)
		So(cursor.fragments.Len(), ShouldEqual, 1)
		formed := cursor.space
		So(formed.Formed, ShouldBeTrue)
		cursor.Step(nil)
		So(cursor.fragments.Len(), ShouldEqual, 2)
		So(cursor.space, ShouldEqual, formed)
		So(cursor.space.Formed, ShouldBeTrue)

		for step := 0; step < 4*(len(first.observations)+len(second.observations))+4 &&
			rehearsal.Wire().Passes < 3; step++ {
			cursor.Step(nil)
		}
		So(rehearsal.Error(), ShouldBeNil)
		So(rehearsal.Wire().Passes, ShouldEqual, 3)
		So(cursor.fragments == start, ShouldBeTrue)
		So(cursor.learned == learned, ShouldBeTrue)

		// Every call is judged the moment it is made, so nothing is left owing.
		So(rehearsal.Wire().Trained, ShouldBeGreaterThan, 0)
		So(rehearsal.Wire().Trained, ShouldEqual, rehearsal.Wire().Decisions)

		for _, observation := range first.observations {
			for _, measurement := range observation.Measurements {
				So(measurement.Metrics["observed_notional"].Coordinates, ShouldBeNil)
			}
		}

		Convey("Each worker publishes the tape it replays and where its moments are", func() {
			worker := rehearsal.cursors[1]
			worker.pending = []fragment{first}

			// A worker explores, so which observation it first speaks on is not
			// fixed; step until it has spoken rather than assuming when.
			for step := 0; step < 3*len(first.observations) &&
				len(worker.track.Marks) == 0; step++ {
				worker.Step(nil)
			}
			track := rehearsal.Wire().Tracks[1]
			So(len(rehearsal.Wire().Tracks), ShouldEqual, len(rehearsal.cursors))
			So(track.Symbol, ShouldEqual, first.observations[0].Symbol)
			So(int(track.Length), ShouldEqual, len(first.observations))
			So(int(track.Index), ShouldEqual, worker.index)
			So(int(track.Entry), ShouldEqual, first.entry)
			So(int(track.Exit), ShouldEqual, first.exit)
			So(len(track.Steps), ShouldEqual, 1+(len(first.observations)-1)/int(track.Stride))

			for index, step := range track.Steps {
				value, defined := first.observations[index*int(track.Stride)].
					Value(rehearsal.policy.Coordinate)
				So(step.Defined, ShouldEqual, defined)
				So(step.Value, ShouldEqual, value)
			}
			So(len(track.Marks), ShouldBeGreaterThan, 0)
			So(track.Marks[0].Kind, ShouldNotEqual, "wait")
			So(int(track.Marks[0].Index), ShouldBeLessThan, len(first.observations))

			// A call carries its verdict immediately; the tape already answered.
			So(track.Marks[0].Graded, ShouldBeTrue)
			So(track.Marks[0].Verdict, ShouldNotBeBlank)
		})

		Convey("A tape without captured precursors asks the worker nothing", func() {
			rehearsal := rehearsalFixture(t)
			episodes := rehearsalEpisodes(rehearsal, 6)
			tape := mustFragment(t, rehearsal, episodes[0])

			for index := range tape.observations {
				tape.observations[index].Measurements = nil
			}
			cursor := rehearsal.cursors[0]
			cursor.pending = []fragment{tape}

			for step := 0; step < 2*len(tape.observations); step++ {
				cursor.Step(nil)
			}
			So(rehearsal.Error(), ShouldBeNil)
			So(rehearsal.Wire().Passes, ShouldEqual, 2)
			So(rehearsal.Wire().Unsupported, ShouldEqual, 2*len(tape.observations))
			So(rehearsal.Wire().Trained, ShouldEqual, 0)
			So(cursor.member.Decisions, ShouldEqual, 0)
			So(cursor.member.Status(), ShouldEqual, runtime.WAITING)
		})

		Convey("Cancellation stops tape advancement", func() {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			cursor.ctx = ctx
			before := cursor.index
			cursor.Step(nil)
			So(cursor.index, ShouldEqual, before)
		})
	})
}

/*
An unquoted touch used to empty the worker's menu, so the observation passed
without a decision at all. A worker makes calls rather than trades: what the
venue could have executed says nothing about whether it read the tape.
*/
func TestReplayCursorAdvanceIgnoresExecutability(t *testing.T) {
	Convey("A tape carrying no quotes at all still asks the worker for its calls", t, func() {
		rehearsal := rehearsalFixture(t)
		episodes := rehearsalEpisodes(rehearsal, 6)
		tape := mustFragment(t, rehearsal, episodes[0])

		// No touch anywhere: nothing on this tape is executable at any size.
		for index := range tape.observations {
			tape.observations[index].HasAsk = false
			tape.observations[index].HasBid = false
			tape.observations[index].AskQty, tape.observations[index].BidQty = 0, 0
		}
		cursor := rehearsal.cursors[0]
		cursor.pending = []fragment{tape}

		for range 3 * len(tape.observations) {
			cursor.Step(nil)
		}
		So(rehearsal.Error(), ShouldBeNil)
		So(cursor.member.Decisions, ShouldBeGreaterThan, 0)
		So(rehearsal.Wire().Decisions, ShouldBeGreaterThan, 0)
		So(rehearsal.Wire().Trained, ShouldEqual, rehearsal.Wire().Decisions)
	})

	Convey("Quote-only tape events cannot invent a precursor", t, func() {
		for _, missing := range []string{"absent", "other symbol"} {
			Convey(missing, func() {
				rehearsal := rehearsalFixture(t)
				episodes := rehearsalEpisodes(rehearsal, 6)
				tape := mustFragment(t, rehearsal, episodes[0])
				cursor := rehearsal.cursors[0]
				cursor.fragments = ring.New(1)
				cursor.fragments.Value = tape
				So(cursor.begin(), ShouldBeNil)

				// Exercise both a missing initial row and a gap after a populated row.
				for index := range 4 {
					if index%2 == 0 {
						tape.observations[index].Measurements = nil

						if missing == "other symbol" {
							other := *tape.observations[index+1].Measurements[0]
							other.Label = "ATOM/USD"
							tape.observations[index].Measurements = []*data.Measurement[float64]{&other}
						}
					}
					version, decisions := cursor.space.Version, cursor.member.Decisions
					So(cursor.advance(), ShouldBeNil)
					So(cursor.index, ShouldEqual, index+1)
					So(cursor.session.index, ShouldEqual, index)

					if index%2 == 0 {
						So(cursor.member.Status(), ShouldEqual, runtime.WAITING)
						So(cursor.space.Version, ShouldEqual, version)
						So(cursor.member.Decisions, ShouldEqual, decisions)
					}

					if index%2 != 0 {
						So(cursor.space.Version, ShouldEqual, version+1)
					}
				}
				So(rehearsal.Wire().Unsupported, ShouldBeGreaterThanOrEqualTo, 2)
			})
		}
	})
}

func TestReplayCursorAdvance(t *testing.T) {
	for _, clock := range []string{"venue", "receive", "both"} {
		Convey("Captured "+clock+" times may regress without regressing objective marks", t, func() {
			rehearsal := rehearsalFixture(t)
			episodes := rehearsalEpisodes(rehearsal, 6)
			tape := mustFragment(t, rehearsal, episodes[0])
			origin := tape.observations[0].ReceivedAt

			for index := range tape.observations {
				at := origin.Add(-time.Duration(index) * time.Millisecond)

				if clock == "venue" || clock == "both" {
					tape.observations[index].VenueAt = at
				}

				if clock == "receive" || clock == "both" {
					tape.observations[index].ReceivedAt = at
				}
			}
			cursor := rehearsal.cursors[0]
			cursor.pending = []fragment{tape}
			started := time.Now()
			var ledger reward.Ledger

			for range len(tape.observations) {
				cursor.Step(nil)
				mark, err := cursor.session.Objective()
				So(err, ShouldBeNil)
				So(mark.At.Before(started), ShouldBeFalse)
				_, err = ledger.Measure(*mark)
				So(err, ShouldBeNil)
			}
			So(rehearsal.Error(), ShouldBeNil)
			So(cursor.member.Error(), ShouldBeNil)
			So(rehearsal.Wire().Passes, ShouldEqual, 1)
			So(tape.observations[0].ReceivedAt, ShouldResemble, origin)
		})
	}
}

func TestReplayCursorBegin(t *testing.T) {
	Convey("Replacing a replay agent closes its runtime resources first", t, func() {
		rehearsal := rehearsalFixture(t)
		episodes := rehearsalEpisodes(rehearsal, 6)
		cursor := rehearsal.cursors[0]
		cursor.fragments = ring.New(1)
		cursor.fragments.Value = mustFragment(t, rehearsal, episodes[0])
		So(cursor.begin(), ShouldBeNil)
		So(cursor.member.Close(), ShouldBeNil)
		closed := false
		cursor.member.System = runtime.NewSystem(t.Context(), "previous-agent", func() error {
			closed = true
			return nil
		})
		previous := cursor.member
		So(cursor.begin(), ShouldBeNil)
		So(closed, ShouldBeTrue)
		So(cursor.member, ShouldNotEqual, previous)
		So(cursor.member.Model, ShouldEqual, previous.Model)

		Convey("A close failure prevents replacement and remains visible", func() {
			So(cursor.member.Close(), ShouldBeNil)
			cursor.member.System = runtime.NewSystem(t.Context(), "failed-agent", func() error {
				return errors.New("agent resource close failed")
			})
			previous = cursor.member
			So(cursor.begin(), ShouldNotBeNil)
			So(cursor.member, ShouldEqual, previous)
		})
	})
}

func BenchmarkReplayCursorStep(b *testing.B) {
	for _, shape := range []string{"measured", "mixed", "quotes"} {
		b.Run(shape, func(b *testing.B) {
			rehearsal := rehearsalFixture(b)
			episodes := rehearsalEpisodes(rehearsal, 6)
			cursor := rehearsal.cursors[0]
			tape := mustFragment(b, rehearsal, episodes[0])

			for index := range tape.observations {
				if shape == "quotes" || (shape == "mixed" && index%2 == 0) {
					tape.observations[index].Measurements = nil
				}
			}
			cursor.pending = []fragment{tape}
			b.ReportAllocs()

			for b.Loop() {
				cursor.Step(nil)

				if err := rehearsal.Error(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
