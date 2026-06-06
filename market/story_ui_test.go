package market

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func TestThesisScoreRMS(t *testing.T) {
	Convey("Given measurement snapshots", t, func() {
		score := thesisScoreRMS([]types.Measurement{
			{SNR: 3},
			{SNR: 0},
			{SNR: 4},
		})

		Convey("It should ignore zero SNR values in the RMS", func() {
			So(score, ShouldAlmostEqual, 3.5355339059327378, 0.0001)
		})
	})
}

func TestRequiredEntryScore(t *testing.T) {
	Convey("Given replay friction defaults", t, func() {
		t.Cleanup(viper.Reset)
		viper.Set("trading.entry_edge_multiple", 2.0)
		viper.Set("trading.paper.taker_fee_pct", 0.26)

		required := requiredEntryScore([]types.Measurement{
			{SpreadBPS: 10},
		})

		Convey("It should scale round-trip friction by the entry edge multiple", func() {
			So(required, ShouldAlmostEqual, 124, 0.0001)
		})
	})
}
