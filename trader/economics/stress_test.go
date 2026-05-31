package economics

import (
	"testing"
	"time"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/market/perspectives"
)

func TestStressQuoteShallowsDepth(t *testing.T) {
	convey.Convey("Given stress mode enabled", t, func() {
		originalStress := config.System.ExecutionStressEnabled
		originalFraction := config.System.ExecutionStressDepthFraction
		config.System.ExecutionStressEnabled = true
		config.System.ExecutionStressDepthFraction = 0.5
		t.Cleanup(func() {
			config.System.ExecutionStressEnabled = originalStress
			config.System.ExecutionStressDepthFraction = originalFraction
		})

		quote := broker.Quote{
			Last:     100,
			Bid:      99,
			Ask:      101,
			At:       time.Now(),
			AskDepth: []market.BookLevel{{Price: 101, Qty: 10}},
		}
		stressed := StressQuote(quote, 0, broker.StressRegime{})

		convey.Convey("It should scale visible depth", func() {
			convey.So(stressed.AskDepth[0].Qty, convey.ShouldEqual, 5)
		})
	})
}

func TestAdverseSelectionBPSFromToxicity(t *testing.T) {
	convey.Convey("Given a toxicity measurement", t, func() {
		original := config.System.AdverseSelectionBPS
		config.System.AdverseSelectionBPS = 5
		t.Cleanup(func() { config.System.AdverseSelectionBPS = original })

		bps := AdverseSelectionBPS([]perspectives.Measurement{{
			Source: perspectives.SourceToxicity,
			SNR:    2,
		}})

		convey.Convey("It should scale the penalty", func() {
			convey.So(bps, convey.ShouldEqual, 10)
		})
	})
}
