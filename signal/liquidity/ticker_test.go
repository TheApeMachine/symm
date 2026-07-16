package liquidity

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/tests"
	tickerfixture "github.com/theapemachine/symm/tests/fixtures/ticker"
)

func TestTickerOn(testingTB *testing.T) {
	Convey("Given a liquidity ticker ingestor", testingTB, func() {
		ticker := &Ticker{cache: tickerCache()}
		fixture := tickerfixture.NewFixture(tickerfixture.UPDATE, 3)

		Convey("When fixture frames are replayed", func() {
			tests.Replay(tests.Handlers{"ticker": ticker.On}, fixture.Frames())

			Convey("Then ticker rows should accumulate in cache", func() {
				So(len(tickerRows(ticker.cache)), ShouldEqual, 3)
				So(tickerRows(ticker.cache)[0].Symbol, ShouldEqual, "ALGO/USD")
			})
		})
	})
}
