package market

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/logic"
)

func TestSymbolStateObserve(t *testing.T) {
	Convey("Given a symbol state", t, func() {
		viper.Set("market.story.measurement_max_age", 5*time.Second)
		state := NewSymbolState()
		now := time.Now().UTC()

		bullish := logic.Measurement{
			Source:     logic.SourcePumpDump,
			Symbol:     "BTC/USD",
			Category:   logic.CategoryVerticalIgnition,
			Confidence: 0.8,
			Surprise:   1.5,
			ObservedAt: now,
		}
		bearish := logic.Measurement{
			Source:     logic.SourcePumpDump,
			Symbol:     "BTC/USD",
			Category:   logic.CategoryFadedExhaustion,
			Confidence: 0.7,
			Surprise:   1.2,
			ObservedAt: now.Add(time.Second),
		}

		state.Observe(bullish)

		Convey("It should replace stale evidence with the latest source reading", func() {
			snapshot := state.Observe(bearish)

			So(len(snapshot), ShouldEqual, 1)
			So(snapshot[0].Category, ShouldEqual, logic.CategoryFadedExhaustion)
		})

		Convey("It should drop measurements outside the TTL", func() {
			expired := bullish
			expired.ObservedAt = now.Add(-10 * time.Second)
			state.latest[logic.SourceHawkes] = expired

			snapshot := state.Observe(bearish)

			So(len(snapshot), ShouldEqual, 1)
			So(snapshot[0].Source, ShouldEqual, logic.SourcePumpDump)
		})
	})
}
