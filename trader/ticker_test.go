package trader

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

func TestTickerMeasureUsesRingDo(t *testing.T) {
	Convey("Given queued ticker frames", t, func() {
		crossSection, err := types.NewCrossSection(types.DefaultCrossSectionConfig())

		So(err, ShouldBeNil)

		signal := &Signal{CrossSection: crossSection}
		ticker := NewTicker(signal, make(chan []byte, 1))

		ticker.On([]byte(`{
			"channel":"ticker",
			"type":"update",
			"data":[{"symbol":"BTC/USD","bid":"100","ask":"102","last":"101","volume":1,"timestamp":"2026-07-11T18:00:00Z"}]
		}`))

		Convey("It should drain the ring without panicking", func() {
			So(func() {
				_, _ = ticker.Measure()
			}, ShouldNotPanic)
			So(ticker.ring.Len(), ShouldEqual, 0)
		})
	})
}

func TestTickerMeasurePartialRows(t *testing.T) {
	Convey("Given a ticker update missing last price", t, func() {
		crossSection, err := types.NewCrossSection(types.DefaultCrossSectionConfig())

		So(err, ShouldBeNil)

		signal := &Signal{CrossSection: crossSection}
		ticker := NewTicker(signal, make(chan []byte, 1))

		ticker.On([]byte(`{
			"channel":"ticker",
			"type":"update",
			"data":[{"symbol":"BTC/USD","bid":"100","ask":"102","volume":1,"timestamp":"2026-07-11T18:00:00Z"}]
		}`))

		Convey("It should measure without panicking", func() {
			So(func() {
				_, _ = ticker.Measure()
			}, ShouldNotPanic)
		})
	})
}
