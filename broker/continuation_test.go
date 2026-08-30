package broker

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func testFlowPerspective(flowAligned float64, flowBook float64, at time.Time) types.Perspective {
	perspective := types.Perspective{Symbol: "SYM/USD", Kind: types.KindFlow, At: at}
	perspective.Readings[0] = types.MetricReading{
		Metric:     nmtypes.MustIntern("advisor/flow/flow_aligned_midpoint_return/sample"),
		Value:      flowAligned,
		Defined:    true,
		ObservedAt: at,
	}
	perspective.Readings[1] = types.MetricReading{
		Metric:     nmtypes.MustIntern("advisor/flow/flow_book_alignment/sample"),
		Value:      flowBook,
		Defined:    true,
		ObservedAt: at,
	}
	perspective.Count = 2

	return perspective
}

type staticReader struct {
	perspectives map[types.PerspectiveKey]types.Perspective
}

func (reader staticReader) Latest(key types.PerspectiveKey) (types.Perspective, bool) {
	perspective, found := reader.perspectives[key]

	return perspective, found
}

/*
TestContinuationFreshness proves the causal-freshness gate: a continuation
context whose reading was observed in the FUTURE relative to the current market
observation, or has no observation instant, never suppresses the profit-
stagnation exit. An old reading held in the store cannot keep a position alive
indefinitely.
*/
func TestContinuationFreshness(t *testing.T) {
	Convey("Given a flow perspective with future-dated alignment", t, func() {
		now := time.Unix(1000, 0)
		future := time.Unix(2000, 0)

		reader := staticReader{perspectives: map[types.PerspectiveKey]types.Perspective{
			{Symbol: "SYM/USD", Kind: types.KindFlow}: testFlowPerspective(0.4, 0.2, future),
		}}

		Convey("fresh rejects a future-dated observation", func() {
			So(fresh(future, now), ShouldBeFalse)
		})

		Convey("continuation does not defer when the flow reading is future-dated", func() {
			entry := positionEntryContext{}
			So(continuationSupportive(reader, entry, "SYM/USD", now), ShouldBeFalse)
		})
	})

	Convey("Given a flow perspective with current alignment", t, func() {
		now := time.Unix(1000, 0)
		past := time.Unix(900, 0)

		Convey("fresh accepts a causally prior (non-future) observation", func() {
			So(fresh(past, now), ShouldBeTrue)
		})

		Convey("fresh rejects a zero observation instant", func() {
			So(fresh(time.Time{}, now), ShouldBeFalse)
		})
	})
}
