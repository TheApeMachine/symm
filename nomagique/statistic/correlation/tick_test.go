package correlation_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	nmcorrelation "github.com/theapemachine/symm/nomagique/statistic/correlation"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestTickNext(t *testing.T) {
	Convey("Tick assembles keyed last and timestamp inputs into a price observation", t, func() {
		origin := transport.NewAddress[string]()
		origin.Identify("ETH/USD")
		last := any(101.5)
		at := any(int64(1_700_000_000_000_000_001))
		lastInput := core.NewInput[string, []string, any](
			origin, core.Write, []string{"ticker", "data", "last"}, &last,
		)
		atInput := core.NewInput[string, []string, any](
			origin, core.Write, []string{"ticker", "data", "timestamp"}, &at,
		)
		out := tests.CollectSeq[nmcorrelation.PriceObservation](
			nmcorrelation.NewTick().Next(sequence.NewValue(*lastInput, *atInput)),
		)
		So(len(out), ShouldEqual, 1)
		So(out[0].Symbol, ShouldEqual, "ETH/USD")
		So(out[0].Value, ShouldEqual, 101.5)
		So(out[0].At, ShouldEqual, int64(1_700_000_000_000_000_001))
	})
}
