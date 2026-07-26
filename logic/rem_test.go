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
		tree, _ := dmt.NewTree("")
		rem := newREMSleep(context.Background(), tree)
		sequence := []byte("symbol-eth-usd_pressure-positive")
		from := time.Unix(0, 100)
		through := time.Unix(0, 200)
		_, ok := tree.CommitToEpisodicBuffer(uint64(from.UnixNano()), sequence)
		So(ok, ShouldBeTrue)
		_, ok = tree.CommitToEpisodicBuffer(uint64(through.UnixNano()), sequence)
		So(ok, ShouldBeTrue)

		rem.Accumulate([]time.Time{from, through})
		So(rem.Pending(), ShouldEqual, 2)

		Convey("When REM is requested", func() {
			rem.Request(7)
			So(rem.Pending(), ShouldEqual, 0)
			rem.Await()

			Convey("Then sensory weights train without holding the caller", func() {
				So(tree.GetSensoryWeight(sequence).Count, ShouldEqual, 2)
				thesis := types.NewThesis()
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
}

func BenchmarkREMSleepAccumulate(b *testing.B) {
	at := time.Unix(0, 1)

	for b.Loop() {
		rem := newREMSleep(context.Background(), nil)
		rem.Accumulate([]time.Time{at})
	}
}
