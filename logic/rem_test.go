package logic

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/types"
)

func TestREMSleepRequestAsync(t *testing.T) {
	Convey("Given pending episodic observations behind the ambiguity gate", t, func() {
		tree := dmt.NewTree("")
		rem := newREMSleep(context.Background(), tree)
		sequence := []byte("symbol-eth-usd_pressure-positive")
		from := time.Unix(0, 100)
		through := time.Unix(0, 200)
		_, _, err := tree.CommitToEpisodicBuffer(uint64(from.UnixNano()), sequence)
		So(err, ShouldBeNil)
		_, _, err = tree.CommitToEpisodicBuffer(uint64(through.UnixNano()), sequence)
		So(err, ShouldBeNil)

		rem.Accumulate([]time.Time{from, through})
		So(rem.Pending(), ShouldEqual, 2)

		Convey("When REM is requested", func() {
			rem.Request(7)
			So(rem.Pending(), ShouldEqual, 0)
			rem.Await()

			Convey("Then sensory weights train without holding the caller", func() {
				So(tree.GetSensoryWeight(sequence).Count, ShouldEqual, 2)
				thesis := types.NewThesis(nil)
				thesis.Cognition.Store("ETH/USD", types.Cognition{Symbol: "ETH/USD"})
				rem.Stamp(thesis)
				reading, _ := thesis.Cognition.Load("ETH/USD")
				cognition := reading.(types.Cognition)
				So(cognition.REMReplays, ShouldEqual, 2)
				So(cognition.REMFrom, ShouldEqual, from)
				So(cognition.REMThrough, ShouldEqual, through)
			})
		})
	})

	Convey("Given REM cannot consume its durable episodic interval", t, func() {
		tree := dmt.NewTree(t.TempDir())
		sequence := []byte("symbol-eth-usd_pressure-positive")
		from := time.Unix(0, 100)
		through := time.Unix(0, 200)
		_, _, err := tree.CommitToEpisodicBuffer(uint64(from.UnixNano()), sequence)
		So(err, ShouldBeNil)
		_, _, err = tree.CommitToEpisodicBuffer(uint64(through.UnixNano()), sequence)
		So(err, ShouldBeNil)
		So(tree.Close(), ShouldBeNil)
		rem := newREMSleep(t.Context(), tree)
		rem.Accumulate([]time.Time{from, through})

		Convey("The failed pass remains pending and is not stamped complete", func() {
			rem.Request(7)
			rem.Await()
			So(rem.Pending(), ShouldEqual, 2)
			thesis := types.NewThesis(nil)
			thesis.Cognition.Store("ETH/USD", types.Cognition{Symbol: "ETH/USD"})
			rem.Stamp(thesis)
			reading, _ := thesis.Cognition.Load("ETH/USD")
			cognition := reading.(types.Cognition)
			So(cognition.REMReplays, ShouldEqual, 0)
			So(cognition.REMFrom.IsZero(), ShouldBeTrue)
			So(cognition.REMThrough.IsZero(), ShouldBeTrue)
			rem.Close()
			So(rem.Pending(), ShouldEqual, 0)
		})
	})
}

/*
TestREMSleepClose proves shutdown joins current consolidation, discards its
queued rerun, and rejects observations arriving after cancellation.
*/
func TestREMSleepClose(t *testing.T) {
	Convey("Given consolidation in flight with a queued rerun", t, func() {
		rem := newREMSleep(t.Context(), dmt.NewTree(""))
		from := time.Unix(0, 100)
		through := time.Unix(0, 200)
		rem.Accumulate([]time.Time{from, through})
		finished := make(chan struct{})
		release := make(chan struct{})

		rem.mu.Lock()
		rem.busy = true
		rem.rerunRequested = true
		rem.rerunTick = 9
		rem.finished = finished
		rem.mu.Unlock()

		go func() {
			<-release
			rem.mu.Lock()
			rem.busy = false
			rem.mu.Unlock()
			close(finished)
		}()

		closed := make(chan struct{})

		go func() {
			rem.Close()
			close(closed)
		}()

		<-rem.ctx.Done()

		select {
		case <-closed:
			t.Fatal("REM Close returned before consolidation finished")
		default:
		}

		close(release)

		select {
		case <-closed:
		case <-t.Context().Done():
			t.Fatal("REM Close did not return after consolidation finished")
		}

		rem.Accumulate([]time.Time{time.Unix(0, 300)})
		rem.Request(10)

		Convey("Then no queued or post-cancel work can launch", func() {
			So(rem.Pending(), ShouldEqual, 0)
			So(rem.busy, ShouldBeFalse)
			So(rem.rerunRequested, ShouldBeFalse)
			So(rem.rerunTick, ShouldEqual, 0)
		})
	})
}

func BenchmarkREMSleepAccumulate(b *testing.B) {
	at := time.Unix(0, 1)

	for b.Loop() {
		rem := newREMSleep(context.Background(), nil)
		rem.Accumulate([]time.Time{at})
	}
}
