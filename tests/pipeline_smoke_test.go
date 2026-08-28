package tests

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/cmd"
	"github.com/theapemachine/symm/nomagique/runtime"
	tes "github.com/theapemachine/symm/tests/types"
	"github.com/theapemachine/symm/types"
)

/*
TestPipelinePumpsFramesToUIBus boots the real system against the fixture
venue and drives real ticks through the bus, asserting the UI channel
receives frames. It is the end-to-end guard for the dashboard data path:
if any stage errors and cancels the bus, or the ingest stops publishing,
this test fails loudly instead of a silent dead dashboard.
*/
func TestPipelinePumpsFramesToUIBus(t *testing.T) {
	symbol := tes.NewSymbol("SMOKE/USD", 1.0, 10)

	Convey("Given the assembled system driven by the fixture venue", t,
		WithStack(t, []*tes.Symbol{symbol}, cmd.Boot, func(market *Market, system *cmd.System) {
			received := make(chan *types.UIFrame, 512)
			runtime.RegisterSink(system.Bus, nil, func(frame *types.UIFrame) {
				if frame == nil {
					return
				}

				select {
				case received <- frame:
				default:
				}
			})

			for index := 0; index < 60; index++ {
				market.Tick()
			}

			deadline := time.Now().Add(5 * time.Second)

			for len(received) == 0 && time.Now().Before(deadline) {
				time.Sleep(10 * time.Millisecond)
			}

			Convey("UI frames should reach the bus from the live pipeline", func() {
				So(len(received), ShouldBeGreaterThan, 0)
			})
		}),
	)
}