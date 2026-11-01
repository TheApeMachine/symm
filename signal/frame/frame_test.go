package frame

import (
	"sync/atomic"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/market"
)

func TestSymbolsEach(t *testing.T) {
	Convey("Given a mixed-symbol trade frame", t, func() {
		now := time.Now().UTC()
		frame := NewSymbols([]*market.TradeData{
			{Symbol: "AAA/USD", Timestamp: now},
			{Symbol: "BBB/USD", Timestamp: now.Add(time.Millisecond)},
			{Symbol: "AAA/USD", Timestamp: now.Add(2 * time.Millisecond)},
			nil,
			{Symbol: "", Timestamp: now},
		})

		var seen atomic.Int32
		order := make(chan []time.Time, 2)

		frame.Each(func(trades []*market.TradeData) {
			seen.Add(1)

			stamps := make([]time.Time, 0, len(trades))

			for _, trade := range trades {
				stamps = append(stamps, trade.Timestamp)
			}

			order <- stamps
		})

		Convey("It fans one batch per symbol and keeps AAA prints in frame order", func() {
			So(seen.Load(), ShouldEqual, 2)

			batches := [][]time.Time{<-order, <-order}
			var aaa []time.Time

			for _, batch := range batches {
				if len(batch) == 2 {
					aaa = batch
				}
			}

			So(aaa, ShouldNotBeNil)
			So(aaa[0].Before(aaa[1]), ShouldBeTrue)
		})
	})
}
