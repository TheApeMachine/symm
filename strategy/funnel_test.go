package strategy

import (
	"errors"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/runtime"
)

func TestFunnelKey(t *testing.T) {
	Convey("funnelKey", t, func() {
		So(funnelKey("board", "board score below floor (score=0.1 floor=0.35 cut=0.4)"),
			ShouldEqual, "board: board score below floor")
		So(funnelKey("", ""), ShouldEqual, "?: ?")
	})
}

func TestFunnelObserve(t *testing.T) {
	Convey("funnel tallies skips and enter failures", t, func() {
		funnel := newFunnel()
		envelope := runtime.NewEnvelope(nil)
		envelope.Decide(&runtime.Decision{
			Symbol: "AAA/USD",
			Action: "skip",
			Gate:   "oracle",
			Reason: "oracle cold — no approve lit",
			Tier:   "real",
		})
		envelope.Decide(&runtime.Decision{
			Symbol: "BBB/USD",
			Action: "skip",
			Gate:   "board",
			Reason: "board score below floor (score=0.1 floor=0.35 cut=0.4)",
			Tier:   "real",
		})

		funnel.failedEnter(errors.New("allocate failed why quote cash unusable"))
		funnel.observe(envelope, 2, 12.5, 0)

		funnel.mu.Lock()
		defer funnel.mu.Unlock()
		// emit clears counters on the first observe (lastEmit zero).
		So(funnel.ticks, ShouldEqual, 1)
		So(funnel.skips, ShouldBeEmpty)
		So(funnel.enterFail, ShouldBeEmpty)
	})
}
