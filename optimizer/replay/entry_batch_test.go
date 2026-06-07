package replay

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestReplayEntryBatchRanking(t *testing.T) {
	Convey("Given batched replay entries", t, func() {
		base := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
		ledger := newReplayLedger(ReplayCosts{PositionFraction: 0.5, StartingCapital: 200})
		ledger.entryBatch = []batchedReplayEntry{
			{
				measurement: types.Measurement{Symbol: "AAVE/EUR", SNR: 1, Confidence: 0.5, At: base},
				conviction:  0.5,
			},
			{
				measurement: types.Measurement{Symbol: "BTC/EUR", SNR: 4, Confidence: 0.9, At: base},
				conviction:  3.6,
			},
		}
		ledger.entryBatchDeadline = base.Add(-time.Millisecond)

		ledger.flushEntryBatch(base.Add(time.Millisecond))

		Convey("It should clear the batch after the window closes", func() {
			So(len(ledger.entryBatch), ShouldEqual, 0)
		})
	})
}

func TestMeasurementConviction(t *testing.T) {
	Convey("Given a measurement", t, func() {
		score := measurementConviction(types.Measurement{SNR: 2, Confidence: 0.5})

		Convey("It should multiply SNR by confidence", func() {
			So(score, ShouldEqual, 1)
		})
	})
}

func TestReplayEntryBatchQueue(t *testing.T) {
	Convey("Given an entry act", t, func() {
		ledger := newReplayLedger(DefaultReplayCosts())
		act := reasoning.Act{Type: reasoning.ActionMarket}
		measurement := types.Measurement{
			Symbol: "BTC/EUR", Last: 100, SNR: 2, Confidence: 0.8,
			At: time.Now(),
		}

		ledger.queueEntryAction(act, measurement, nil)

		Convey("It should hold the act until the batch window closes", func() {
			So(len(ledger.entryBatch), ShouldEqual, 1)
		})
	})
}
