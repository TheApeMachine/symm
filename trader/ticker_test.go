package trader

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	tickerfixture "github.com/theapemachine/symm/tests/fixtures/ticker"
	"github.com/theapemachine/symm/types"
)

func TestTickerMeasure(t *testing.T) {
	Convey("Given a ticker producer that replenishes the ring during measurement", t, func() {
		capacity := viper.GetInt("signals.feed_ring_capacity")
		defer viper.Set("signals.feed_ring_capacity", capacity)
		viper.Set("signals.feed_ring_capacity", 8)

		frame := tickerFrame()
		signal := &replenishingSignal{}
		ticker := NewTicker(&Signal{Ticker: []types.Signal[any]{signal}}, nil)
		signal.replenish = func() { ticker.On(frame) }
		ticker.On(frame)

		_, err := ticker.Measure()

		Convey("It should consume only the prefix present when the cycle began", func() {
			So(err, ShouldBeNil)
			So(ticker.ring.Len(), ShouldEqual, 1)
		})
	})

	Convey("Given a ticker update missing its last price", t, func() {
		capacity := viper.GetInt("signals.feed_ring_capacity")
		defer viper.Set("signals.feed_ring_capacity", capacity)
		viper.Set("signals.feed_ring_capacity", 8)

		crossSection, err := types.NewCrossSection(types.DefaultCrossSectionConfig())
		So(err, ShouldBeNil)

		ticker := NewTicker(
			&Signal{CrossSection: crossSection},
			make(chan []byte, 1),
		)
		ticker.On([]byte(`{
			"channel":"ticker",
			"type":"update",
			"data":[{"symbol":"BTC/USD","bid":"100","ask":"102","volume":1,"timestamp":"2026-07-11T18:00:00Z"}]
		}`))

		Convey("It should process the partial row without panicking", func() {
			So(func() {
				_, _ = ticker.Measure()
			}, ShouldNotPanic)
		})
	})
}

func tickerFrame() []byte {
	for frame := range tickerfixture.NewFixture(tickerfixture.SNAPSHOT, 1).Generate() {
		return frame
	}

	return nil
}
