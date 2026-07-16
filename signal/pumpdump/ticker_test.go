package pumpdump

import (
	"errors"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura/structure"
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

			Convey("Then falling behind reports the exact retained boundary", func() {
				_, err := ticker.cache.Pending(time.Now().UTC())
				overrun := structure.ClockOverrunError{}

				So(errors.As(err, &overrun), ShouldBeTrue)
				So(overrun.Expected, ShouldEqual, 1)
				So(overrun.Oldest, ShouldEqual, 2)
			})
		})
	})
}
