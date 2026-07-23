package hawkes_test

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/stack"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/types"
)

func TestDebugColdHawkes(t *testing.T) {
	Convey("debug cold", t, func() {
		market := tests.NewMarket(t.Context(), 3)
		wired, err := stack.NewBooter(t.Context()).Test(market)
		So(err, ShouldBeNil)
		defer wired.Close()
		defer market.Close()

		var last *types.Thesis
		err = market.Transition(tests.MarketStateBaseline, func() error {
			thesis, err := wired.Observe()
			if err != nil {
				return err
			}
			last = thesis
			hawkes, other := 0, 0
			for _, m := range thesis.Measurements {
				if m == nil {
					continue
				}
				if m.Source == types.SourceHawkes {
					hawkes++
				} else {
					other++
				}
			}
			fmt.Printf("observe: total=%d hawkes=%d other=%d symbols=%v\n",
				len(thesis.Measurements), hawkes, other, wired.Signals[len(wired.Signals)-1])
			return nil
		})
		So(err, ShouldBeNil)
		So(last, ShouldNotBeNil)
		fmt.Printf("final measurements=%d\n", len(last.Measurements))
		for _, m := range last.Measurements {
			if m != nil && m.Source == types.SourceHawkes {
				fmt.Printf("  hawkes %s %s %s raw=%v\n", m.Symbol, m.Metric, m.Side, m.Raw)
			}
		}
		// also check hawkes process state
		for _, sig := range wired.Signals {
			if sig.Name() == string(types.SourceHawkes) {
				fmt.Printf("hawkes signal symbols from Name ok\n")
			}
		}
	})
}
