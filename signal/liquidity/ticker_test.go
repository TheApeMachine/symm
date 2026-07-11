package liquidity

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func liquidityRow(
	symbol string, bid, ask, bidQty, askQty, volume, vwap float64,
) kraken.TickerData {
	return kraken.TickerData{
		Symbol:    symbol,
		Bid:       decimal.NewFromFloat64(bid),
		BidQty:    bidQty,
		Ask:       decimal.NewFromFloat64(ask),
		AskQty:    askQty,
		Last:      decimal.NewFromFloat64((bid + ask) / 2),
		Volume:    volume,
		Vwap:      vwap,
		Timestamp: time.Now(),
	}
}

func liquidityCrossSection(t *testing.T, rows ...kraken.TickerData) *types.CrossSection {
	t.Helper()

	crossSection, err := types.NewCrossSection(types.DefaultCrossSectionConfig())
	So(err, ShouldBeNil)
	So(crossSection.Observe(rows), ShouldBeNil)

	return crossSection
}

func TestTickerMeasureRequiresCrossSection(t *testing.T) {
	Convey("Given a ticker signal with no cross-section", t, func() {
		ticker := NewTicker()

		Convey("When Measure runs", func() {
			result, err := ticker.Measure(kraken.TickerData{}, nil)

			Convey("Then it returns a validation error", func() {
				So(err, ShouldNotBeNil)
				So(result, ShouldBeNil)
			})
		})
	})
}

func TestTickerMeasureRequiresTwoExecutablePeers(t *testing.T) {
	Convey("Given a cross-section with only one observed symbol", t, func() {
		ticker := NewTicker()
		subject := liquidityRow("BTC/USD", 999, 1001, 2, 2, 1, 1000)
		crossSection := liquidityCrossSection(t, subject)

		Convey("When Measure runs", func() {
			result, err := ticker.Measure(subject, crossSection)

			Convey("Then it emits nothing rather than dividing by an unsupported median", func() {
				So(err, ShouldBeNil)
				So(result, ShouldBeNil)
			})
		})
	})
}

func TestTickerMeasureUsesExecutableValueNotRawVolume(t *testing.T) {
	Convey("Given two penny-priced peers with huge raw volume but tiny quote notional", t, func() {
		ticker := NewTicker()

		peerOne := liquidityRow("PENNY1/USD", 0.0001, 0.0001, 1_000_000, 1_000_000, 1_000_000, 0.0001)
		peerTwo := liquidityRow("PENNY2/USD", 0.0001, 0.0001, 1_000_000, 1_000_000, 1_000_000, 0.0001)

		Convey("And a subject with tiny raw volume but a high-value, deep quote", func() {
			// Raw Volume is 1, dwarfed by the peers' 1,000,000 — a raw-volume
			// comparison would call this extreme scarcity. Its quote notional
			// and executable depth are both far above the peer median.
			subject := liquidityRow("BTC/USD", 999, 1001, 2, 2, 1, 1000)
			crossSection := liquidityCrossSection(t, subject, peerOne, peerTwo)

			Convey("When Measure runs", func() {
				result, err := ticker.Measure(subject, crossSection)

				Convey("Then it scores the subject as liquid, not scarce", func() {
					So(err, ShouldBeNil)
					So(result, ShouldHaveLength, 1)

					measurement := result[0]
					So(measurement.Metrics["rvol"], ShouldBeGreaterThan, 1)
					So(measurement.Metrics["depthScore"], ShouldBeGreaterThan, 0)
					So(measurement.Metrics["scarcityScore"], ShouldEqual, 0)

					var robust, scarce types.Category

					for _, category := range measurement.Categories {
						if category.Type == types.RobustLiquidity {
							robust = category
						}

						if category.Type == types.ExtremeScarcity {
							scarce = category
						}
					}

					So(robust.Strength, ShouldBeGreaterThan, scarce.Strength)
				})
			})
		})
	})
}

func TestTickerMeasureAtPeerMedianIsBalanced(t *testing.T) {
	Convey("Given a subject whose notional and depth match its peers exactly", t, func() {
		ticker := NewTicker()

		peerOne := liquidityRow("ETH/USD", 99, 101, 5, 5, 100, 100)
		peerTwo := liquidityRow("SOL/USD", 99, 101, 5, 5, 100, 100)
		subject := liquidityRow("BTC/USD", 99, 101, 5, 5, 100, 100)
		crossSection := liquidityCrossSection(t, subject, peerOne, peerTwo)

		Convey("When Measure runs", func() {
			result, err := ticker.Measure(subject, crossSection)

			Convey("Then relative value is one and neither scarcity nor depth dominates", func() {
				So(err, ShouldBeNil)
				So(result, ShouldHaveLength, 1)

				measurement := result[0]
				So(measurement.Metrics["rvol"], ShouldAlmostEqual, 1, 1e-9)
				So(measurement.Metrics["scarcityScore"], ShouldEqual, 0)
				So(measurement.Metrics["depthScore"], ShouldEqual, 0)
				So(measurement.Metrics["medianScore"], ShouldEqual, 1)
			})
		})
	})
}

func TestTickerMeasureSkipsNonExecutableSubject(t *testing.T) {
	Convey("Given a subject with a one-sided quote (no bid) among executable peers", t, func() {
		ticker := NewTicker()

		peerOne := liquidityRow("ETH/USD", 99, 101, 5, 5, 100, 100)
		peerTwo := liquidityRow("SOL/USD", 99, 101, 5, 5, 100, 100)
		subject := kraken.TickerData{
			Symbol:    "BTC/USD",
			Bid:       decimal.NewFromFloat64(0),
			BidQty:    0,
			Ask:       decimal.NewFromFloat64(101),
			AskQty:    5,
			Last:      decimal.NewFromFloat64(101),
			Volume:    100,
			Vwap:      100,
			Timestamp: time.Now(),
		}
		crossSection := liquidityCrossSection(t, subject, peerOne, peerTwo)

		Convey("When Measure runs", func() {
			result, err := ticker.Measure(subject, crossSection)

			Convey("Then it emits nothing rather than pricing an unexecutable quote", func() {
				So(err, ShouldBeNil)
				So(result, ShouldBeNil)
			})
		})
	})
}

func BenchmarkTickerMeasure(b *testing.B) {
	ticker := NewTicker()
	crossSection, err := types.NewCrossSection(types.DefaultCrossSectionConfig())

	if err != nil {
		b.Fatal(err)
	}

	peerOne := liquidityRow("ETH/USD", 99, 101, 5, 5, 100, 100)
	peerTwo := liquidityRow("SOL/USD", 99, 101, 5, 5, 100, 100)
	subject := liquidityRow("BTC/USD", 999, 1001, 2, 2, 1, 1000)

	if err := crossSection.Observe([]kraken.TickerData{subject, peerOne, peerTwo}); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		if _, err := ticker.Measure(subject, crossSection); err != nil {
			b.Fatal(err)
		}
	}
}
