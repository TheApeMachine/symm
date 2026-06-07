package market

import (
	"container/ring"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/internal/testconfig"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestRingSnapshotPerSymbolHistory(t *testing.T) {
	Convey("Given a story with per-symbol measurement windows", t, func() {
		testconfig.Load(t)

		story := &Story{
			symbolWindows:  make(map[string]*ring.Ring),
			windowCapacity: 128,
		}

		Convey("Interleaved updates from many symbols retain full history per symbol", func() {
			for step := range 40 {
				for symbolIndex := range 50 {
					story.rememberMeasurement(types.Measurement{
						Symbol: fmt.Sprintf("SYM%d/EUR", symbolIndex),
						Last:   100 + float64(step),
						At:     time.Unix(int64(step), 0).UTC(),
					})
				}
			}

			snapshots := story.ringSnapshot("SYM0/EUR")

			So(len(snapshots), ShouldEqual, 40)

			context := reasoning.NewWindowReason(
				snapshots,
				types.RegimeTrending,
				reasoning.PositionState{},
			)

			now, okNow := context.Scalar(reasoning.SubjectPrice, reasoning.UnitPercentage, reasoning.Lookback{Ago: 0})
			then, okThen := context.Scalar(reasoning.SubjectPrice, reasoning.UnitPercentage, reasoning.Lookback{Ago: 30})

			So(okNow, ShouldBeTrue)
			So(okThen, ShouldBeTrue)
			So(now, ShouldEqual, 139)
			So(then, ShouldEqual, 109)
		})
	})
}
