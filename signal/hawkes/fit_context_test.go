package hawkes

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken/trade"
)

func TestNewFitContext(t *testing.T) {
	convey.Convey("Given sparse and dense arrival streams", t, func() {
		sparse := fitContextForEventCount(t, 16)
		dense := fitContextForEventCount(t, 64)

		convey.Convey("It should raise the minimum event floor with density", func() {
			convey.So(dense.MinFitEvents, convey.ShouldBeGreaterThan, sparse.MinFitEvents)
			convey.So(sparse.MinFitEvents, convey.ShouldBeGreaterThanOrEqualTo, bivariateParamCount*2)
		})

		convey.Convey("It should raise the branch ceiling toward criticalBranch with sample size", func() {
			denseCeiling := fitContextForEventCount(t, 256)

			convey.So(denseCeiling.BranchCeiling, convey.ShouldBeGreaterThan, sparse.BranchCeiling)
			convey.So(sparse.BranchCeiling, convey.ShouldBeLessThan, criticalBranch)
			convey.So(denseCeiling.BranchCeiling, convey.ShouldBeLessThan, criticalBranch)
		})

		convey.Convey("It should widen the trade window for slower or denser streams", func() {
			convey.So(dense.TradeWindow, convey.ShouldBeGreaterThan, sparse.TradeWindow)
		})

		convey.Convey("It should derive positive scan bounds from event gaps", func() {
			start := time.Unix(0, 0)
			buyEvents, sellEvents := balancedBurstEvents(start, 16, 6, 50*time.Millisecond)
			horizon := buyEvents[len(buyEvents)-1].Add(10 * time.Millisecond)
			stream := NewArrivalStream(buyEvents, sellEvents)
			context, ok := NewFitContext(stream, horizon)

			convey.So(ok, convey.ShouldBeTrue)
			convey.So(context.MinFitEvents, convey.ShouldBeGreaterThan, 0)
			convey.So(context.MinPerSide, convey.ShouldBeGreaterThan, 0)
			convey.So(len(context.BetaCandidates), convey.ShouldBeGreaterThanOrEqualTo, 3)
			convey.So(context.TradeWindow, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestFitContextFromTicks(t *testing.T) {
	start := time.Unix(0, 0)
	buyEvents := burstEvents(start, 16, 40*time.Millisecond)
	sellEvents := sparseSellEvents(start.Add(-time.Second), 6)
	ticks := make([]trade.Data, 0, len(buyEvents)+len(sellEvents))

	for _, eventTime := range buyEvents {
		ticks = append(ticks, trade.Data{Side: "buy", Timestamp: eventTime})
	}

	for _, eventTime := range sellEvents {
		ticks = append(ticks, trade.Data{Side: "sell", Timestamp: eventTime})
	}

	horizon := buyEvents[len(buyEvents)-1].Add(50 * time.Millisecond)

	convey.Convey("Given a long tick history and a short horizon", t, func() {
		context, _, ok := FitContextFromTicks(ticks, time.Time{}, horizon)

		convey.Convey("It should adapt the trade window from probe context", func() {
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(context.TradeWindow, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestFitContextEnoughEvents(t *testing.T) {
	start := time.Unix(0, 0)
	buyEvents := burstEvents(start, 16, 40*time.Millisecond)
	sellEvents := burstEvents(start.Add(time.Millisecond), 16, 45*time.Millisecond)
	horizon := buyEvents[len(buyEvents)-1].Add(100 * time.Millisecond)
	stream := NewArrivalStream(buyEvents, sellEvents)

	convey.Convey("Given a balanced bivariate stream", t, func() {
		context, ok := NewFitContext(stream, horizon)

		convey.So(ok, convey.ShouldBeTrue)

		convey.Convey("It should accept streams meeting both per-side floors", func() {
			convey.So(context.EnoughEvents(stream), convey.ShouldBeTrue)
		})
	})
}
