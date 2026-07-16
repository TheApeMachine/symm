package pumpdump

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	tickerfixture "github.com/theapemachine/symm/tests/fixtures/ticker"
	"github.com/theapemachine/symm/types"
)

func TestTickerOn(testingTB *testing.T) {
	Convey("Given a pumpdump ticker ingestor", testingTB, func() {
		ticker := &Ticker{
			cache: types.NewMarketFeed[kraken.TickerData](2, 2),
		}
		fixture := tickerfixture.NewFixture(tickerfixture.UPDATE, 3)

		Convey("When fixture frames are replayed", func() {
			tests.Replay(tests.Handlers{"ticker": ticker.On}, fixture.Frames())

			Convey("Then the current retained window remains available", func() {
				rows, err := ticker.cache.Pending(time.Now().UTC())

				So(err, ShouldBeNil)
				So(rows, ShouldHaveLength, 2)
			})
		})
	})
}
