package strategy

import (
	"container/ring"
	"context"
	"errors"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"testing"
)

func TestReplayCursorStep(t *testing.T) {
	Convey("A learner advances one observation or completed grade per workload step", t, func() {
		rehearsal := rehearsalFixture(t)
		episodes := rehearsalEpisodes(rehearsal)
		first := mustFragment(t, rehearsal, episodes[0])
		second := mustFragment(t, rehearsal, episodes[1])
		cursor := rehearsal.cursors[0]
		cursor.pending = [][]hindsight.Observation{first}
		learned := cursor.learned
		cursor.Step(nil)
		So(rehearsal.Error(), ShouldBeNil)
		So(cursor.index, ShouldEqual, 1)
		So(rehearsal.Wire().Trained, ShouldEqual, 0)
		start := cursor.fragments
		cursor.pending = [][]hindsight.Observation{second}

		for step := 0; step < 2*len(first)+2 && rehearsal.Wire().Passes < 1; step++ {
			cursor.Step(nil)
		}
		So(rehearsal.Error(), ShouldBeNil)
		So(rehearsal.Wire().Passes, ShouldEqual, 1)
		So(cursor.fragments.Len(), ShouldEqual, 1)
		cursor.Step(nil)
		So(cursor.fragments.Len(), ShouldEqual, 2)

		for step := 0; step < 4*(len(first)+len(second))+4 && rehearsal.Wire().Passes < 3; step++ {
			cursor.Step(nil)
		}
		So(rehearsal.Error(), ShouldBeNil)
		So(rehearsal.Wire().Passes, ShouldEqual, 3)
		So(cursor.fragments == start, ShouldBeTrue)
		So(cursor.learned == learned, ShouldBeTrue)
		So(rehearsal.Wire().Trained, ShouldBeGreaterThan, 0)
		So(rehearsal.Wire().Trained, ShouldEqual, rehearsal.Wire().Decisions)

		for _, observation := range first {
			for _, measurement := range observation.Measurements {
				So(measurement.Metrics["observed_notional"].Coordinates, ShouldBeNil)
			}
		}

		Convey("Each worker publishes the tape it is actually replaying", func() {
			worker := rehearsal.cursors[1]
			worker.pending = [][]hindsight.Observation{first}

			for step := 0; step < len(first) && len(worker.track.Marks) == 0; step++ {
				worker.Step(nil)
			}
			track := rehearsal.Wire().Tracks[1]
			So(len(rehearsal.Wire().Tracks), ShouldEqual, len(rehearsal.cursors))
			So(track.Symbol, ShouldEqual, first[0].Symbol)
			So(int(track.Length), ShouldEqual, len(first))
			So(int(track.Index), ShouldEqual, worker.index)
			So(len(track.Steps), ShouldEqual, 1+(len(first)-1)/int(track.Stride))

			for index, step := range track.Steps {
				value, defined := first[index*int(track.Stride)].Value(rehearsal.policy.Coordinate)
				So(step.Defined, ShouldEqual, defined)
				So(step.Value, ShouldEqual, value)
			}
			So(len(track.Marks), ShouldEqual, 1)
			So(track.Marks[0].Kind, ShouldNotEqual, "wait")
			So(int(track.Marks[0].Index), ShouldBeLessThan, len(first))
			So(track.Marks[0].Graded, ShouldBeFalse)

			for step := 0; step < 2*len(first) &&
				!rehearsal.Wire().Tracks[1].Marks[0].Graded; step++ {
				worker.Step(nil)
			}
			graded := rehearsal.Wire().Tracks[1].Marks[0]
			So(graded.Graded, ShouldBeTrue)
			So(graded.Id, ShouldEqual, track.Marks[0].Id)
		})

		Convey("A tape without captured precursors completes without training", func() {
			rehearsal := rehearsalFixture(t)
			episodes := rehearsalEpisodes(rehearsal)
			fragment := mustFragment(t, rehearsal, episodes[0])

			for index := range fragment {
				fragment[index].Measurements = nil
			}
			cursor := rehearsal.cursors[0]
			cursor.pending = [][]hindsight.Observation{fragment}

			for step := 0; step < 2*len(fragment); step++ {
				cursor.Step(nil)
			}
			So(rehearsal.Error(), ShouldBeNil)
			So(rehearsal.Wire().Passes, ShouldEqual, 2)
			So(rehearsal.Wire().Unsupported, ShouldEqual, 2*len(fragment))
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

func TestReplayCursorAdvance(t *testing.T) {
	Convey("Missing executable depth gates the agent while measurements still reach the grid", t, func() {
		rehearsal := rehearsalFixture(t)
		episodes := rehearsalEpisodes(rehearsal)
		fragment := mustFragment(t, rehearsal, episodes[0])
		fragment[0].HasAsk = false
		cursor := rehearsal.cursors[0]
		cursor.fragments = ring.New(1)
		cursor.fragments.Value = fragment
		So(cursor.begin(), ShouldBeNil)
		So(cursor.advance(), ShouldBeNil)
		So(cursor.space.Version, ShouldEqual, 1)
		So(cursor.member.Status(), ShouldEqual, runtime.WAITING)
		So(cursor.member.Decisions, ShouldEqual, 0)
		So(cursor.lastProfit, ShouldBeNil)
		So(cursor.advance(), ShouldBeNil)
		So(cursor.space.Version, ShouldEqual, 2)
		So(cursor.lastAt, ShouldResemble, fragment[1].ReceivedAt)
	})

	Convey("Quote-only tape events update economics without inventing a precursor", t, func() {
		for _, missing := range []string{"absent", "other symbol"} {
			Convey(missing, func() {
				rehearsal := rehearsalFixture(t)
				episodes := rehearsalEpisodes(rehearsal)
				fragment := mustFragment(t, rehearsal, episodes[0])
				cursor := rehearsal.cursors[0]
				cursor.fragments = ring.New(1)
				cursor.fragments.Value = fragment
				So(cursor.begin(), ShouldBeNil)

				// Exercise both a missing initial row and a gap after a populated row.
				for index := range 4 {
					if index%2 == 0 {
						fragment[index].Measurements = nil

						if missing == "other symbol" {
							other := *fragment[index+1].Measurements[0]
							other.Label = "ATOM/USD"
							fragment[index].Measurements = []*data.Measurement[float64]{&other}
						}
					}
					version, decisions := cursor.space.Version, cursor.member.Decisions
					So(cursor.advance(), ShouldBeNil)
					So(cursor.index, ShouldEqual, index+1)
					So(cursor.trader.At, ShouldResemble, fragment[index].ReceivedAt)
					So(cursor.lastAt, ShouldResemble, fragment[index].ReceivedAt)

					if index%2 == 0 {
						So(cursor.member.Status(), ShouldEqual, runtime.WAITING)
						So(cursor.space.Version, ShouldEqual, version)
						So(cursor.member.Decisions, ShouldEqual, decisions)
						So(rehearsal.Wire().Trained, ShouldEqual, 0)
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

func TestReplayCursorBegin(t *testing.T) {
	Convey("Replacing a replay agent closes its runtime resources first", t, func() {
		rehearsal := rehearsalFixture(t)
		episodes := rehearsalEpisodes(rehearsal)
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
			episodes := rehearsalEpisodes(rehearsal)
			cursor := rehearsal.cursors[0]
			fragment := mustFragment(b, rehearsal, episodes[0])

			for index := range fragment {
				if shape == "quotes" || (shape == "mixed" && index%2 == 0) {
					fragment[index].Measurements = nil
				}
			}
			cursor.pending = [][]hindsight.Observation{fragment}
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
