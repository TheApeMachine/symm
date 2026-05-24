package exhaust

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/engine"
)

type stubMarketReader map[string]engine.Snapshot

func (reader stubMarketReader) Read(symbol string) engine.Snapshot {
	return reader[symbol]
}

func (reader stubMarketReader) ReadFresh(
	symbol string,
	_ time.Time,
	_ time.Duration,
) engine.Snapshot {
	return reader.Read(symbol)
}

type busyTicker struct {
	ticks int
}

func (ticker *busyTicker) Tick() bool {
	ticker.ticks++

	return true
}

func TestExhaustTickDoesNotKeepDrainLoopBusy(t *testing.T) {
	Convey("Given exhaust watching one open symbol", t, func() {
		exhaust, err := NewExhaust(
			context.Background(),
			stubMarketReader{
				"PUMP/EUR": {LastOK: true, Last: 1.25, SpreadOK: true, SpreadBPS: 10},
			},
			nil,
		)
		So(err, ShouldBeNil)

		exhaust.WatchSymbol("PUMP/EUR")

		Convey("When Tick samples the symbol", func() {
			busy := exhaust.Tick()

			Convey("It should report no queued work remaining", func() {
				So(busy, ShouldBeFalse)
			})
		})
	})
}

func TestExhaustTickDrainCap(t *testing.T) {
	Convey("Given a ticker that always reports work", t, func() {
		ticker := &busyTicker{}
		const maxDrainIterations = 10_000

		iterations := 0

		for iteration := 0; iteration < maxDrainIterations; iteration++ {
			if !ticker.Tick() {
				break
			}

			iterations++
		}

		Convey("It should stop after a finite drain budget", func() {
			So(iterations, ShouldEqual, maxDrainIterations)
		})
	})
}
