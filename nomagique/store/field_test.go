package store_test

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/core"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
	"github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
)

func TestFieldNext(t *testing.T) {
	Convey("Field yields the matching keyed float and ignores the rest", t, func() {
		origin := transport.NewAddress[string]()
		origin.Identify("ETH/USD")
		last := any(101.5)
		bid := any(100.0)
		lastInput := core.NewInput[string, []string, any](
			origin, core.Write, []string{"ticker", "data", "last"}, &last,
		)
		bidInput := core.NewInput[string, []string, any](
			origin, core.Write, []string{"ticker", "data", "bid"}, &bid,
		)
		out := tests.CollectSeq[float64](
			store.NewField("ticker", "data", "last").Next(sequence.NewValue(*lastInput, *bidInput)),
		)
		So(out, ShouldResemble, []float64{101.5})
	})
}

func TestOriginNext(t *testing.T) {
	Convey("Origin yields inputs only for the configured symbol", t, func() {
		eth := transport.NewAddress[string]()
		eth.Identify("ETH/USD")
		btc := transport.NewAddress[string]()
		btc.Identify("BTC/USD")
		last := any(1.0)
		ethInput := core.NewInput[string, []string, any](
			eth, core.Write, []string{"ticker", "data", "last"}, &last,
		)
		btcInput := core.NewInput[string, []string, any](
			btc, core.Write, []string{"ticker", "data", "last"}, &last,
		)
		out := tests.CollectSeq[core.Input[string, []string, any]](
			store.NewOrigin("ETH/USD").Next(sequence.NewValue(*ethInput, *btcInput)),
		)
		So(len(out), ShouldEqual, 1)
		So(out[0].Origin.Identity(), ShouldEqual, "ETH/USD")
	})
}
