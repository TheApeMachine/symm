package strategy

import (
	"context"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/hindsight"
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

func BenchmarkReplayCursorStep(b *testing.B) {
	rehearsal := rehearsalFixture(b)
	episodes := rehearsalEpisodes(rehearsal)
	cursor := rehearsal.cursors[0]
	cursor.pending = [][]hindsight.Observation{mustFragment(b, rehearsal, episodes[0])}
	b.ReportAllocs()
	for b.Loop() {
		cursor.Step(nil)
		if err := rehearsal.Error(); err != nil {
			b.Fatal(err)
		}
	}
}
