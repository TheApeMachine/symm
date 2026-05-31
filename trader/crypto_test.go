package trader

import (
	"context"
	"fmt"
	"testing"

	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/focus"
	decision "github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/trader/economics"
	"github.com/theapemachine/symm/wallet"
)

func newTestCrypto() *Crypto {
	ctx := context.Background()

	return &Crypto{
		ctx:       ctx,
		wallet:    wallet.NewWallet(wallet.PaperWallet, "EUR", 200, 0.26),
		tracker:   focus.NewSet(),
		story:     decision.NewStory(),
		positions: newPositionBook(),
		quotes:    newQuoteCache(),
		economics: economics.NewDesk(),
		paper:     NewPaperSession(ctx),
		makers:    newMakerDesk(),
		readings:  make(map[string]map[perspectives.SourceType]timedMeasurement),
	}
}

func traderMeasurement(
	symbol string,
	source perspectives.SourceType,
	category perspectives.CategoryType,
	snr float64,
) perspectives.Measurement {
	return perspectives.Measurement{
		Symbol:   symbol,
		Source:   source,
		Category: category,
		SNR:      snr,
		Last:     100,
	}
}

func TestCryptoRecord(t *testing.T) {
	convey.Convey("Given two contradictory categories from the same source", t, func() {
		crypto := newTestCrypto()
		crypto.record(traderMeasurement(
			"BTC/EUR",
			perspectives.SourceCVD,
			perspectives.CategoryAggressiveDrive,
			2.0,
		))
		crypto.record(traderMeasurement(
			"BTC/EUR",
			perspectives.SourceCVD,
			perspectives.CategoryStochasticBalance,
			0.2,
		))

		convey.Convey("It should keep only the newest category for that source", func() {
			measurements := crypto.snapshot("BTC/EUR")

			convey.So(measurements, convey.ShouldHaveLength, 1)
			convey.So(measurements[0].Category, convey.ShouldEqual, perspectives.CategoryStochasticBalance)
		})
	})
}

func TestCryptoMarketCalibrated(t *testing.T) {
	convey.Convey("Given the whole observed market emits the same entry signal", t, func() {
		crypto := newTestCrypto()

		for index := range 20 {
			symbol := fmt.Sprintf("COIN%d/EUR", index)
			crypto.record(traderMeasurement(
				symbol,
				perspectives.SourceCVD,
				perspectives.CategoryAggressiveDrive,
				2.0,
			))
		}

		candidate, ok := crypto.entryOpportunity("COIN0/EUR", crypto.snapshot("COIN0/EUR"))

		convey.Convey("It should identify the local playbook but reject it as non-distinct", func() {
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(crypto.marketCalibrated(candidate), convey.ShouldBeFalse)
		})
	})

	convey.Convey("Given one symbol stands above the live cross-section", t, func() {
		crypto := newTestCrypto()

		for index := range 19 {
			symbol := fmt.Sprintf("COIN%d/EUR", index)
			crypto.record(traderMeasurement(
				symbol,
				perspectives.SourceCVD,
				perspectives.CategoryStochasticBalance,
				0.2,
			))
		}

		crypto.record(traderMeasurement(
			"LEADER/EUR",
			perspectives.SourceCVD,
			perspectives.CategoryAggressiveDrive,
			3.0,
		))

		candidate, ok := crypto.entryOpportunity("LEADER/EUR", crypto.snapshot("LEADER/EUR"))
		calibrated, allowed := crypto.calibrateOpportunity(candidate)

		convey.Convey("It should allow the outlier and assign it positive edge", func() {
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(allowed, convey.ShouldBeTrue)
			convey.So(calibrated.Edge, convey.ShouldBeGreaterThan, 0)
		})
	})
}

func TestCryptoOpportunityShare(t *testing.T) {
	convey.Convey("Given two live opportunities with unequal edge", t, func() {
		crypto := newTestCrypto()

		for index := range 18 {
			symbol := fmt.Sprintf("COIN%d/EUR", index)
			crypto.record(traderMeasurement(
				symbol,
				perspectives.SourceCVD,
				perspectives.CategoryStochasticBalance,
				0.2,
			))
		}

		crypto.record(traderMeasurement(
			"FAST/EUR",
			perspectives.SourceCVD,
			perspectives.CategoryAggressiveDrive,
			4.0,
		))
		crypto.record(traderMeasurement(
			"SLOW/EUR",
			perspectives.SourceCVD,
			perspectives.CategoryAggressiveDrive,
			2.0,
		))

		fast, _ := crypto.entryOpportunity("FAST/EUR", crypto.snapshot("FAST/EUR"))
		slow, _ := crypto.entryOpportunity("SLOW/EUR", crypto.snapshot("SLOW/EUR"))
		fast, _ = crypto.calibrateOpportunity(fast)
		slow, _ = crypto.calibrateOpportunity(slow)

		convey.Convey("It should give the stronger candidate the larger wallet share", func() {
			convey.So(crypto.opportunityShare(fast), convey.ShouldBeGreaterThan, crypto.opportunityShare(slow))
		})
	})
}

func BenchmarkCryptoMarketCalibrated(b *testing.B) {
	crypto := newTestCrypto()

	for index := range 64 {
		symbol := fmt.Sprintf("COIN%d/EUR", index)
		strength := 0.2

		if index == 7 {
			strength = 3.0
		}

		category := perspectives.CategoryStochasticBalance

		if index == 7 {
			category = perspectives.CategoryAggressiveDrive
		}

		crypto.record(traderMeasurement(symbol, perspectives.SourceCVD, category, strength))
	}

	candidate, _ := crypto.entryOpportunity("COIN7/EUR", crypto.snapshot("COIN7/EUR"))

	for b.Loop() {
		_ = crypto.marketCalibrated(candidate)
	}
}
