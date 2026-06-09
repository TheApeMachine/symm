package correlation

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
)

func TestCrossSectionSymbolFresh(t *testing.T) {
	Convey("Given a symbol with a stale last tick", t, func() {
		viper.Set("signals.trade_match_window", time.Minute)
		crossSection, err := newCrossSection(4, 16)
		So(err, ShouldBeNil)
		crossSection.publishPrice("BTC/EUR", 100, time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
		state, _ := crossSection.universe.Load("BTC/EUR")
		symbolState := state.(*symbolState)
		symbolState.lastTickAt = time.Now().Add(-5 * time.Minute)

		Convey("It should treat the symbol as stale", func() {
			So(crossSection.symbolFresh(symbolState, time.Now()), ShouldBeFalse)
			So(crossSection.symbolAge("BTC/EUR", time.Now()), ShouldBeGreaterThan, time.Minute)
		})
	})
}
