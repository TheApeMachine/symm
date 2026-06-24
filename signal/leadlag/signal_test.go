package leadlag

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/signal/testutil"
)

func leadlagTestPool(testingTB testing.TB) *qpool.Q[any] {
	if testingTB != nil {
		testingTB.Helper()
	}

	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)

	if pool == nil && testingTB != nil {
		testingTB.Fatal("qpool.NewQ returned nil")
	}

	return pool
}

var leadlagCategories = []logic.CategoryType{
	logic.CategoryInefficientLag,
	logic.CategorySynchronizedDrift,
	logic.CategoryDecoupledMove,
	logic.CategoryAnchorStall,
}

func categoryResult(result *datura.Artifact) int {
	return testutil.DominantCategoryIndex(result, leadlagCategories)
}

var classifierInputs = []string{"inefficient", "sync", "decoupled", "stall"}

func outputScore(result *datura.Artifact, key string) float64 {
	return datura.Peek[float64](result, "output", key)
}

func winningClassifierInput(result *datura.Artifact) string {
	bestKey := classifierInputs[0]
	bestScore := outputScore(result, bestKey)

	for _, key := range classifierInputs[1:] {
		score := outputScore(result, key)

		if score > bestScore {
			bestScore = score
			bestKey = key
		}
	}

	return bestKey
}

func tickerDatapoint(symbol string, last float64, timestamp int64) *datura.Artifact {
	payload := fmt.Sprintf(
		`{"channel":"ticker","type":"update","data":[{"symbol":%q,"last":%g,"volume":1000,"change_pct":0.1}]}`,
		symbol, last,
	)

	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("ticker")
	artifact.WithScope("update")
	artifact.WithPayload([]byte(payload))
	artifact.SetTimestamp(timestamp)

	return artifact
}

func TestSectionPriceSamples(testingTB *testing.T) {
	Convey("Given ticker observations", testingTB, func() {
		section := NewSection("BTC/EUR")
		start := time.Now()

		for index := range 20 {
			section.ObservePrice(
				"BTC/EUR",
				100+float64(index),
				start.Add(time.Duration(index)*ringSampleSpacing),
			)
		}

		Convey("It should retain enough samples for correlation", func() {
			So(section.PriceSampleCount("BTC/EUR"), ShouldBeGreaterThanOrEqualTo, minLagSamples)
		})
	})
}

func TestSectionCrossLagInsufficientData(testingTB *testing.T) {
	Convey("Given sparse histories", testingTB, func() {
		section := NewSection("BTC/EUR")
		now := time.Now()

		section.ObservePrice("BTC/EUR", 100, now)
		section.ObservePrice("ETH/EUR", 200, now)

		features := section.Features("ETH/EUR")

		Convey("It should refuse to score lag without enough samples", func() {
			So(features.LagOK, ShouldBeFalse)
		})
	})
}

func TestRecentPathMove(testingTB *testing.T) {
	Convey("Given a flat anchor path across the lag window", testingTB, func() {
		start := time.Now()
		samples := make([]priceSample, minLagSamples)

		for index := range minLagSamples {
			samples[index] = priceSample{
				at:    start.Add(time.Duration(index) * 2 * time.Minute),
				value: 50000,
			}
		}

		move, ok := recentPathMove(samples, time.Duration(maxLagBars)*barInterval)

		Convey("It should report a near-zero move", func() {
			So(ok, ShouldBeTrue)
			So(move, ShouldBeLessThan, 1e-6)
		})
	})
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given anchor impulse with a lagging follower", testingTB, func() {
		viper.Set("market.anchor_symbol", "BTC/USD")

		signal := NewSignal(context.Background(), leadlagTestPool(testingTB), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		const (
			flatSamples  = 200
			trackSamples = 140
			spikeSamples = 20
			lagDelay     = 120
		)
		start := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
		totalSamples := flatSamples + trackSamples + spikeSamples

		for index := range flatSamples {
			at := start.Add(time.Duration(index) * ringSampleSpacing)
			signal.Section.ObservePrice("BTC/USD", 50000, at)
			signal.Section.ObservePrice("ETH/USD", 3000, at)
		}

		for range 13 {
			_ = signal.Section.Features("BTC/USD")
		}

		for index := range trackSamples {
			global := flatSamples + index
			at := start.Add(time.Duration(global) * ringSampleSpacing)
			anchorPrice := 50000.0 + float64(index)*5
			followerPrice := 3000.0

			if index >= lagDelay {
				followerPrice = 3000.0 + float64(index-lagDelay)*0.0012
			}

			signal.Section.ObservePrice("BTC/USD", anchorPrice, at)
			signal.Section.ObservePrice("ETH/USD", followerPrice, at)
			_ = signal.Section.Features("ETH/USD")
		}

		trackAnchorEnd := 50000.0 + float64(trackSamples-1)*5

		for index := range spikeSamples {
			global := flatSamples + trackSamples + index
			at := start.Add(time.Duration(global) * ringSampleSpacing)
			anchorPrice := trackAnchorEnd + float64(index+1)*2500

			signal.Section.ObservePrice("BTC/USD", anchorPrice, at)
			signal.Section.ObservePrice("ETH/USD", 3000, at)
			_ = signal.Section.Features("ETH/USD")
		}

		followerFrame := tickerDatapoint(
			"ETH/USD",
			3000,
			start.Add(time.Duration(totalSamples)*ringSampleSpacing).UnixNano(),
		)
		result := signal.Measure(followerFrame)
		followerFrame.Release()

		Convey("It should classify inefficient lag with inefficient winning", func() {
			So(result, ShouldNotBeNil)
			So(categoryResult(result), ShouldEqual, logic.CategoryIndex(logic.CategoryInefficientLag))
			So(outputScore(result, "inefficient"), ShouldBeGreaterThan, outputScore(result, "sync"))
			So(winningClassifierInput(result), ShouldEqual, "inefficient")
			So(outputScore(result, "confidence"), ShouldBeGreaterThan, 0.25)
		})
	})
}
