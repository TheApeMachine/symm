package replay

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestReplayMeasurementsSnapshot(t *testing.T) {
	Convey("Given replay measurement rows", t, func() {
		measurements := newReplayMeasurements()
		measurements.Add(types.Measurement{
			Source: types.SourceFluid,
			Symbol: "BTC/EUR",
			SNR:    1,
		})
		measurements.Add(types.Measurement{
			Source: types.SourceExhaustion,
			SNR:    2,
		})

		Convey("It should merge global and symbol rows", func() {
			rows := measurements.Snapshot("BTC/EUR")

			So(len(rows), ShouldEqual, 2)
		})
	})
}

func TestHalfSpreadSlippagePct(t *testing.T) {
	Convey("Given spread-aware replay costs", t, func() {
		costs := DefaultReplayCosts()

		Convey("It should halve quoted spread into slippage", func() {
			So(halfSpreadSlippagePct(costs, 20), ShouldAlmostEqual, 0.001, 0.0001)
		})

		Convey("It should fall back to static slippage without spread", func() {
			So(halfSpreadSlippagePct(costs, 0), ShouldEqual, costs.SlippagePct)
		})
	})
}

func TestAcquireReplayLedger(t *testing.T) {
	Convey("Given pooled replay ledgers", t, func() {
		ledger := acquireReplayLedger(DefaultReplayCosts())
		releaseReplayLedger(ledger)

		Convey("It should reset ledger state on acquire", func() {
			So(ledger, ShouldNotBeNil)
		})
	})
}

func BenchmarkReplayMeasurementsSnapshot(b *testing.B) {
	measurements := newReplayMeasurements()
	measurements.Add(types.Measurement{Source: types.SourceSentiment})
	measurements.Add(types.Measurement{
		Symbol: "BTC/EUR",
		Source: types.SourceHawkes,
	})

	for b.Loop() {
		_ = measurements.Snapshot("BTC/EUR")
	}
}

func BenchmarkReplayMeasurementsAdd(b *testing.B) {
	measurements := newReplayMeasurements()
	row := types.Measurement{Source: types.SourceFluid, Symbol: "BTC/EUR", SNR: 1}

	for b.Loop() {
		measurements.Add(row)
	}
}
