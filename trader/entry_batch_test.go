package trader

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
)

func TestRankEntryCandidates(t *testing.T) {
	Convey("Given concurrent entry candidates", t, func() {
		weak := reasoning.Action{Symbol: "AAVE/EUR", SNR: 1.0, Confidence: 0.5}
		strong := reasoning.Action{Symbol: "BTC/EUR", SNR: 4.0, Confidence: 0.9}

		ranked := rankEntryCandidates([]reasoning.Action{weak, strong})

		Convey("It should order highest conviction first", func() {
			So(ranked[0].Symbol, ShouldEqual, "BTC/EUR")
			So(ranked[1].Symbol, ShouldEqual, "AAVE/EUR")
		})
	})
}

func TestEntryBatchFlush(t *testing.T) {
	Convey("Given a trader collecting concurrent entry signals", t, func() {
		crypto := newTestCrypto()
		crypto.capitalBase = 200
		crypto.availableQuote = 200
		crypto.positionFraction = 0.5

		weak := reasoning.Action{
			Type: reasoning.ActionMarket, Symbol: "AAVE/EUR",
			Side: trading.Buy, Price: 10, SNR: 1.0, Confidence: 0.5,
		}
		strong := reasoning.Action{
			Type: reasoning.ActionMarket, Symbol: "BTC/EUR",
			Side: trading.Buy, Price: 100, SNR: 4.0, Confidence: 0.9,
		}

		crypto.queueEntry(weak)
		crypto.queueEntry(strong)
		crypto.entryBatch.deadline = time.Now().Add(-time.Millisecond)

		ranked := rankEntryCandidates(crypto.entryBatch.candidates)

		Convey("It ranks the batch before deployment", func() {
			So(ranked[0].Symbol, ShouldEqual, "BTC/EUR")
			So(len(crypto.entryBatch.candidates), ShouldEqual, 2)
		})
	})
}

func TestEntryBatchPreemption(t *testing.T) {
	Convey("Given a full wallet and a stronger late signal", t, func() {
		viper.Set("trading.entry.preemption_enabled", true)
		defer viper.Set("trading.entry.preemption_enabled", true)

		crypto := newTestCrypto()
		crypto.capitalBase = 200
		crypto.availableQuote = 200
		crypto.positionFraction = 1.0
		crypto.inventory["AAVE/EUR"] = 1
		crypto.entryConviction["AAVE/EUR"] = 0.2

		strong := reasoning.Action{
			Type: reasoning.ActionMarket, Symbol: "BTC/EUR",
			Side: trading.Buy, Price: 100, SNR: 5.0, Confidence: 1.0,
		}

		victim, victimScore, ok := crypto.weakestHeldPosition()

		Convey("It should identify the weakest held slot for preemption", func() {
			So(ok, ShouldBeTrue)
			So(victim, ShouldEqual, "AAVE/EUR")
			So(actionConviction(strong), ShouldBeGreaterThan, victimScore)
		})
	})
}

func BenchmarkActionConviction(b *testing.B) {
	action := reasoning.Action{SNR: 2.5, Confidence: 0.8}

	b.ReportAllocs()

	for b.Loop() {
		_ = actionConviction(action)
	}
}

func BenchmarkRankEntryCandidates(b *testing.B) {
	candidates := make([]reasoning.Action, 16)

	for index := range candidates {
		candidates[index] = reasoning.Action{
			Symbol:     "SYM/EUR",
			SNR:        float64(index),
			Confidence: 0.5,
		}
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = rankEntryCandidates(candidates)
	}
}
