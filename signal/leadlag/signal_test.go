package leadlag

import (
	"context"
	"iter"
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

func leadlagTestSpacing() time.Duration {
	return 15 * time.Second
}

var leadlagCategories = []logic.CategoryType{
	logic.CategoryInefficientLag,
	logic.CategorySynchronizedDrift,
	logic.CategoryDecoupledMove,
	logic.CategoryAnchorStall,
}

func categoryResult(result *datura.Artifact) int {
	return dominantCategoryIndex(result, leadlagCategories)
}

func outputScore(result *datura.Artifact, key string) float64 {
	return datura.Peek[float64](result, "output", key)
}

func observePeer(
	t testing.TB,
	crossSection *market.CrossSection,
	symbol string,
	price float64,
	change float64,
	at time.Time,
) {
	t.Helper()

	if err := crossSection.Observe(kraken.TickerDataSlice{{
		Symbol:    symbol,
		Last:      price,
		Volume:    1000,
		ChangePct: change * 100,
		Timestamp: at,
	}}); err != nil {
		t.Fatal(err)
	}
}

func TestSectionPriceSamples(testingTB *testing.T) {
	Convey("Given ticker observations", testingTB, func() {
		section := NewSection()
		section.SetAnchor("BTC/EUR")
		start := time.Now()

		for index := range 20 {
			section.ObservePrice(
				"BTC/EUR",
				100+float64(index),
				start.Add(time.Duration(index)*leadlagTestSpacing()),
			)
		}

		Convey("It should retain enough samples for correlation", func() {
			So(section.PriceSampleCount("BTC/EUR"), ShouldBeGreaterThanOrEqualTo, minCorrelationSamples(16))
		})
	})
}

func TestSectionCrossLagInsufficientData(testingTB *testing.T) {
	Convey("Given sparse histories", testingTB, func() {
		section := NewSection()
		section.SetAnchor("BTC/EUR")
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
		samples := make([]priceSample, minCorrelationSamples(16))

		for index := range minCorrelationSamples(16) {
			samples[index] = priceSample{
				at:    start.Add(time.Duration(index) * 2 * time.Minute),
				value: 50000,
			}
		}

		move, ok := recentPathMove(samples, medianSampleSpacing(samples)*time.Duration(maxLagBarsForCount(16)))

		Convey("It should report a near-zero move", func() {
			So(ok, ShouldBeTrue)
			So(move, ShouldBeLessThan, 1e-6)
		})
	})
}

func TestSignalFirstObservationEmitsLowConfidence(testingTB *testing.T) {
	Convey("Given only two spaced ticks on anchor and follower", testingTB, func() {
		crossSection, csErr := market.NewCrossSection(market.DefaultCrossSectionConfig())
		So(csErr, ShouldBeNil)

		base := time.Date(2026, 6, 11, 11, 0, 0, 0, time.UTC)
		leaderRows := []struct {
			name   string
			change float64
		}{{"BTC/USD", 0.9}, {"ETH/USD", 0.01}, {"SOL/USD", 0.012}}

		for index, sample := range leaderRows {
			observePeer(
				testingTB,
				crossSection,
				sample.name,
				100+float64(index),
				sample.change,
				base.Add(time.Duration(index)*time.Minute),
			)
		}

		So(crossSection.Leader(), ShouldEqual, "BTC/USD")

		signal := NewSignal(context.Background(), dmt.NewTree(""))

		defer func() {
			_ = signal.Close()
		}()

		signal.Section.SetAnchor("BTC/USD")
		start := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)

		// pearsonFloor+1 price samples yield pearsonFloor returns — the structural
		// minimum for a correlation, not a warmup gate. The signal must emit
		// evidence on this first observation, not nil.
		sampleCount := pearsonFloor + 1

		for index := range sampleCount {
			at := start.Add(time.Duration(index) * leadlagTestSpacing())
			signal.Section.ObservePrice("BTC/USD", 50000+float64(index)*10, at)
			signal.Section.ObservePrice("ETH/USD", 3000+float64(index)*0.5, at)
		}

		followerFrame := tickerDatapoint(
			"ETH/USD",
			3000,
			0,
			start.Add(time.Duration(sampleCount)*leadlagTestSpacing()).UnixNano(),
		)
		result := firstMeasured(signal.Measure(followerFrame, crossSection))
		followerFrame.Release()

		Convey("It should emit evidence on the first observation, not nil", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "confidence"), ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given anchor impulse with a lagging follower", testingTB, func() {
		crossSection, csErr := market.NewCrossSection(market.DefaultCrossSectionConfig())
		So(csErr, ShouldBeNil)

		// Make BTC/USD the live cross-section leader so it becomes the anchor.
		anchorBase := time.Date(2026, 6, 11, 11, 0, 0, 0, time.UTC)
		leaderRows := []struct {
			name   string
			change float64
		}{{"BTC/USD", 0.9}, {"ETH/USD", 0.01}, {"SOL/USD", 0.012}}

		for index, sample := range leaderRows {
			observePeer(
				testingTB,
				crossSection,
				sample.name,
				100+float64(index),
				sample.change,
				anchorBase.Add(time.Duration(index)*time.Minute),
			)
		}

		So(crossSection.Leader(), ShouldEqual, "BTC/USD")

		signal := NewSignal(context.Background(), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		// In production Measure sets the anchor every tick before observing;
		// the test feeds history in bulk, so seed the anchor up front.
		signal.Section.SetAnchor("BTC/USD")

		const (
			flatSamples  = 200
			trackSamples = 140
			spikeSamples = 20
		)
		totalSamples := flatSamples + trackSamples + spikeSamples
		lagDelay := maxLagBarsForCount(totalSamples) - 1
		start := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)

		for index := range flatSamples {
			at := start.Add(time.Duration(index) * leadlagTestSpacing())
			signal.Section.ObservePrice("BTC/USD", 50000, at)
			signal.Section.ObservePrice("ETH/USD", 3000, at)
		}

		for range 13 {
			_ = signal.Section.Features("BTC/USD")
		}

		for index := range trackSamples {
			global := flatSamples + index
			at := start.Add(time.Duration(global) * leadlagTestSpacing())
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
			at := start.Add(time.Duration(global) * leadlagTestSpacing())
			anchorPrice := trackAnchorEnd + float64(index+1)*2500

			signal.Section.ObservePrice("BTC/USD", anchorPrice, at)
			signal.Section.ObservePrice("ETH/USD", 3000, at)
			_ = signal.Section.Features("ETH/USD")
		}

		followerFrame := tickerDatapoint(
			"ETH/USD",
			3000,
			0,
			start.Add(time.Duration(totalSamples)*leadlagTestSpacing()).UnixNano(),
		)
		result := firstMeasured(signal.Measure(followerFrame, crossSection))
		followerFrame.Release()

		Convey("It should classify decoupled move when the follower stalls during anchor spike", func() {
			So(result, ShouldNotBeNil)
			So(categoryResult(result), ShouldEqual, logic.CategoryIndex(logic.CategoryDecoupledMove))
			So(outputScore(result, "decoupled"), ShouldBeGreaterThan, outputScore(result, "sync"))
			So(outputScore(result, "confidence"), ShouldBeGreaterThan, 0.25)
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	base := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	spacing := leadlagTestSpacing()

	crossSection, _ := market.NewCrossSection(market.DefaultCrossSectionConfig())
	observePeer(b, crossSection, "BTC/USD", 50000, 0.9, base)
	observePeer(b, crossSection, "ETH/USD", 3000, 0.01, base)

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), dmt.NewTree(""))

		for index := range 48 {
			at := base.Add(time.Duration(index) * spacing)
			signal.Section.ObservePrice("BTC/USD", 50000+float64(index)*5, at)
			signal.Section.ObservePrice("ETH/USD", 3000+float64(index)*0.01, at)

			frame := tickerDatapoint(
				"ETH/USD",
				3000+float64(index)*0.01,
				0,
				at.UnixNano(),
			)
			measured := firstMeasured(signal.Measure(frame, crossSection))
			frame.Release()

			if measured != nil {
				measured.Release()
			}
		}

		_ = signal.Close()
	}
}

func tickerDatapoint(symbol string, last float64, changePct float64, timestamp int64) *datura.Artifact {
	return tickerDatapointWithVolume(symbol, last, 1000.0, changePct, timestamp)
}

func tickerDatapointWithVolume(symbol string, last float64, volume float64, changePct float64, timestamp int64) *datura.Artifact {
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("ticker")
	artifact.WithScope(symbol)
	artifact.WithPayload(datura.Map[any]{
		"channel": "ticker",
		"type":    "update",
		"data": []datura.Map[any]{
			{
				"symbol":     symbol,
				"last":       last,
				"volume":     volume,
				"change_pct": changePct,
			},
		},
	}.Marshal())
	artifact.SetTimestamp(timestamp)

	return artifact
}

func firstMeasured(measurements iter.Seq[*datura.Artifact]) *datura.Artifact {
	for measurement := range measurements {
		return measurement
	}

	return nil
}

func categoryMass(result *datura.Artifact, category logic.CategoryType) float64 {
	distribution := datura.Peek[map[string]any](result, "output", "distribution")
	mass, _ := distribution[strconv.Itoa(logic.CategoryIndex(category))].(float64)

	return mass
}

func dominantCategoryIndex(result *datura.Artifact, categories []logic.CategoryType) int {
	best := categories[0]
	bestMass := categoryMass(result, best)

	for _, category := range categories[1:] {
		mass := categoryMass(result, category)

		if mass > bestMass {
			best = category
			bestMass = mass
		}
	}

	return logic.CategoryIndex(best)
}
