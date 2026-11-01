package strategy

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/runtime"
)

func TestCompactSymbols(t *testing.T) {
	Convey("compactSymbols sorts and drops duplicates", t, func() {
		So(compactSymbols([]string{"ETH/USD", "BTC/USD", "ETH/USD"}), ShouldResemble, []string{
			"BTC/USD", "ETH/USD",
		})
	})
}

func TestTickLight(t *testing.T) {
	Convey("light tick carries paths and decisions without signal maps", t, func() {
		cut := runtime.NewTokens()
		envelope := runtime.NewEnvelope(nil)
		envelope.Paths["BTC/USD"] = &cut
		envelope.Decide(&runtime.Decision{
			Symbol: "BTC/USD",
			Action: "blocked",
			Reason: "blown-out spread",
			Gate:   "physical",
		})

		tick := &Tick{}
		light := tick.light(envelope)

		So(light.Paths["BTC/USD"], ShouldEqual, "")
		So(light.Decisions["BTC/USD"].Action, ShouldEqual, "blocked")
		So(light.Decisions["BTC/USD"].Reason, ShouldEqual, "blown-out spread")
		So(light.Symbols, ShouldResemble, []string{"BTC/USD"})
	})
}
