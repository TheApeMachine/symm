package broker

import (
	"sync"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/nomagique/runtime"
)

func TestSnapshotHelloConcurrent(t *testing.T) {
	Convey("Setup", t, func() {
		pos := NewPosition(t.Context(), testSymbol, "paper", "path", nil, nil, nil)
		pos.Apply(market.ExecutionData{
			ExecID: "b1", Side: "buy",
			LastQty: dec("1"), LastPrice: dec("100"),
			Timestamp: time.Unix(1, 0).UTC(),
		})
		pos.Transition(runtime.READY)
		pos.Mark = dec("110")
		pos.PnL = dec("10")
		pos.PnLPct = dec("10")
		pos.Stoploss = NewStoploss(10)

		Convey("When Hello snapshots race AppendHoldPath", func() {
			var wg sync.WaitGroup
			wg.Add(2)

			go func() {
				defer wg.Done()
				for i := 0; i < 100; i++ {
					_ = pos.SnapshotHello()
				}
			}()

			go func() {
				defer wg.Done()
				for i := 0; i < 100; i++ {
					pos.AppendHoldPath("R06.R00/R07.R01")
				}
			}()

			wg.Wait()

			Convey("No panic under concurrent read/write", func() {
				snap := pos.SnapshotHello()
				So(snap, ShouldNotBeNil)
				So(snap.ID, ShouldEqual, pos.ID)
			})
		})
	})
}
