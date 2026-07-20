package pumpdump

import (
	"context"
	"iter"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/types"
)

/*
measureMarket drives frames through the injectable websocket Conn and the
production Market, returning one measurement epoch for each ticker update.
*/
func measureMarket(t testing.TB, frames iter.Seq[tests.Frame]) [][]*types.Measurement {
	t.Helper()
	previousTimeline := viper.Get("signals.feed_timeline_capacity")
	previousTrack := viper.Get("signals.feed_track_capacity")
	viper.Set("signals.feed_timeline_capacity", 128)
	viper.Set("signals.feed_track_capacity", 128)
	t.Cleanup(func() {
		viper.Set("signals.feed_timeline_capacity", previousTimeline)
		viper.Set("signals.feed_track_capacity", previousTrack)
	})

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	mock := mockapi.NewMockAPI()
	api := websocket.NewAPI(ctx, mock.Public(), mock.Private(), nil)
	t.Cleanup(api.Close)
	instrument := broker.NewInstrument(api, broker.NewPrice(api), nil)
	api.On("instrument", instrument.On)
	market, err := trader.NewMarket(ctx, api, instrument)
	So(err, ShouldBeNil)
	t.Cleanup(market.Close)
	signal := NewSignal(ctx, api, nil, viper.GetInt("signals.feed_track_capacity"))
	epochs := make([][]*types.Measurement, 0)
	cutAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	for frame := range frames {
		mock.Emit(frame.Channel, frame.Payload)
		cut, cutErr := market.Cut(cutAt)
		So(cutErr, ShouldBeNil)
		cutAt = cutAt.Add(time.Second)

		if cut.IsEmpty() {
			continue
		}

		epoch := signal.Measure(types.NewThesis(nil, cut))

		if frame.Channel == "ticker" && len(epoch) > 0 {
			epochs = append(epochs, epoch)
		}
	}

	return epochs
}

/*
indexEpoch makes the one-measurement-per-metric contract explicit for phase
assertions.
*/
func indexEpoch(epoch []*types.Measurement) map[types.MetricType]*types.Measurement {
	indexed := make(map[types.MetricType]*types.Measurement, len(epoch))

	for _, measurement := range epoch {
		indexed[measurement.Metric] = measurement
	}

	return indexed
}

/*
TestSignal_PumpDumpSimulationDetectsIgnition drives the canonical pump-and-dump
simulation through the production websocket and Market path and proves two
things the per-ticker-advance model failed: the panel never fully collapses to
zero (the live spread is always reported), and the volume-clock ignition and
strength scores actually light up on the pump legs.
*/
func TestSignal_PumpDumpSimulationDetectsIgnition(t *testing.T) {
	Convey("Given the canonical pump-and-dump simulation", t, func() {
		epochs := measureMarket(t, conditions.PumpDump().Frames())

		So(len(epochs), ShouldBeGreaterThan, 0)

		var maxIgnition, maxStrength, maxRVOL, maxExhaustion float64
		fullyZero := 0

		for _, epoch := range epochs {
			indexed := indexEpoch(epoch)
			nonZero := false

			for _, measurement := range epoch {
				if measurement.Raw != 0 {
					nonZero = true
				}
			}

			if !nonZero {
				fullyZero++
			}

			maxIgnition = math.Max(maxIgnition, indexed[types.MetricIgnition].Raw)
			maxStrength = math.Max(maxStrength, indexed[types.MetricStrength].Raw)
			maxRVOL = math.Max(maxRVOL, indexed[types.MetricRVOL].Raw)
			maxExhaustion = math.Max(maxExhaustion, indexed[types.MetricExhaustion].Raw)
		}

		Convey("Then no epoch collapses every metric to zero", func() {
			So(fullyZero, ShouldEqual, 0)
		})

		Convey("Then the pump legs raise ignition, strength, and relative volume", func() {
			So(maxRVOL, ShouldBeGreaterThan, 0)
			So(maxIgnition, ShouldBeGreaterThan, 0)
			So(maxStrength, ShouldBeGreaterThan, 0)
		})

		Convey("Then the dump leg registers exhaustion", func() {
			So(maxExhaustion, ShouldBeGreaterThan, 0)
		})
	})
}

/*
TestSignal_MeasureFromMarket proves every pumpdump measurement across a full
calibration, compression, ignition, continuation, rejection, and re-ignition
cycle delivered through the injected Conn and production Market.
*/
func TestSignal_MeasureFromMarket(t *testing.T) {
	Convey("Given a calibrated synthetic Kraken pump-and-dump tape", t, func() {
		epochs := measureMarket(t, conditions.TapePumpDump().Frames())
		So(len(epochs), ShouldBeGreaterThanOrEqualTo, 7)

		baseline := indexEpoch(epochs[len(epochs)-7])
		compression := indexEpoch(epochs[len(epochs)-6])
		ignition := indexEpoch(epochs[len(epochs)-5])
		continuation := indexEpoch(epochs[len(epochs)-4])
		rejection := indexEpoch(epochs[len(epochs)-3])
		recoiled := indexEpoch(epochs[len(epochs)-2])
		reignition := indexEpoch(epochs[len(epochs)-1])
		phases := []map[types.MetricType]*types.Measurement{
			baseline, compression, ignition, continuation,
			rejection, recoiled, reignition,
		}
		expected := map[types.MetricType]struct {
			subject types.SubjectType
			unit    types.MeasurementUnit
		}{
			types.MetricRVOL: {
				subject: types.SubjectPumpVolumeLift,
				unit:    types.UnitDimensionless,
			},
			types.MetricPrecursor: {
				subject: types.SubjectPumpPriceLift,
				unit:    types.UnitDimensionless,
			},
			types.MetricSpread: {
				subject: types.SubjectPumpSpread,
				unit:    types.UnitQuoteCurrency,
			},
			types.MetricCompression: {
				subject: types.SubjectPumpCompression,
				unit:    types.UnitDimensionless,
			},
			types.MetricIgnition: {
				subject: types.SubjectPumpIgnition,
				unit:    types.UnitDimensionless,
			},
			types.MetricTrend: {
				subject: types.SubjectPumpTrend,
				unit:    types.UnitDimensionless,
			},
			types.MetricExhaustion: {
				subject: types.SubjectPumpExhaustion,
				unit:    types.UnitDimensionless,
			},
			types.MetricStrength: {
				subject: types.SubjectPumpComposite,
				unit:    types.UnitDimensionless,
			},
		}

		Convey("Then every phase publishes the complete measurement contract", func() {
			for _, phase := range phases {
				So(len(phase), ShouldEqual, len(expected))

				for metric, contract := range expected {
					measurement, found := phase[metric]
					So(found, ShouldBeTrue)
					So(measurement.Source, ShouldEqual, types.SourcePumpDump)
					So(measurement.Stream, ShouldEqual, types.PumpDump)
					So(measurement.Subject, ShouldEqual, contract.subject)
					So(measurement.Symbol, ShouldEqual, conditions.Subject())
					So(measurement.Unit, ShouldEqual, contract.unit)
					So(measurement.At.IsZero(), ShouldBeFalse)
					So(math.IsNaN(measurement.Raw), ShouldBeFalse)
					So(math.IsInf(measurement.Raw, 0), ShouldBeFalse)
					So(measurement.Maturity, ShouldBeBetween, 0, 1)
					So(measurement.Validity.State, ShouldEqual, types.ValidityValid)
					So(measurement.Validity.Readiness, ShouldEqual, types.ReadinessObservation)
					So(measurement.Scale.Kind, ShouldEqual, types.ScaleObservationWindow)
					So(measurement.Scale.From, ShouldResemble, measurement.At)
					So(measurement.Scale.Through, ShouldResemble, measurement.At)
					So(measurement.ValidateStruct(), ShouldBeNil)

					if measurement.Raw == 0 {
						So(measurement.Normalized, ShouldBeNil)
						continue
					}

					So(measurement.Normalized, ShouldNotBeNil)

					if measurement.Unit == types.UnitDimensionless {
						So(*measurement.Normalized, ShouldAlmostEqual, measurement.Raw)
						continue
					}

					So(*measurement.Normalized, ShouldBeGreaterThan, 0)
					So(*measurement.Normalized, ShouldBeLessThan, 1)
				}
			}
		})

		Convey("Then each event phase has distinct evidence", func() {
			So(baseline[types.MetricRVOL].Raw, ShouldBeGreaterThan, 0)
			So(baseline[types.MetricPrecursor].Raw, ShouldBeGreaterThan, 0)
			So(baseline[types.MetricCompression].Raw, ShouldEqual, 0)
			So(baseline[types.MetricExhaustion].Raw, ShouldEqual, 0)

			So(compression[types.MetricSpread].Raw,
				ShouldBeLessThan, baseline[types.MetricSpread].Raw)
			So(compression[types.MetricCompression].Raw,
				ShouldBeGreaterThan, baseline[types.MetricCompression].Raw)

			So(ignition[types.MetricRVOL].Raw,
				ShouldBeGreaterThan, compression[types.MetricRVOL].Raw)
			So(ignition[types.MetricPrecursor].Raw,
				ShouldBeGreaterThan, baseline[types.MetricPrecursor].Raw)
			So(ignition[types.MetricIgnition].Raw,
				ShouldBeGreaterThan, baseline[types.MetricIgnition].Raw)
			So(ignition[types.MetricTrend].Raw,
				ShouldBeGreaterThan, baseline[types.MetricTrend].Raw)
			So(ignition[types.MetricExhaustion].Raw, ShouldEqual, 0)

			So(continuation[types.MetricRVOL].Raw,
				ShouldBeGreaterThan, baseline[types.MetricRVOL].Raw)
			So(continuation[types.MetricPrecursor].Raw,
				ShouldBeGreaterThan, baseline[types.MetricPrecursor].Raw)
			So(continuation[types.MetricTrend].Raw, ShouldBeGreaterThan, 0)
			So(continuation[types.MetricExhaustion].Raw, ShouldEqual, 0)

			So(rejection[types.MetricRVOL].Raw,
				ShouldBeLessThan, continuation[types.MetricRVOL].Raw)
			So(rejection[types.MetricPrecursor].Raw, ShouldEqual, 0)
			So(rejection[types.MetricSpread].Raw,
				ShouldBeGreaterThan, baseline[types.MetricSpread].Raw)
			So(rejection[types.MetricCompression].Raw, ShouldEqual, 0)
			So(rejection[types.MetricIgnition].Raw, ShouldEqual, 0)
			So(rejection[types.MetricTrend].Raw, ShouldEqual, 0)
			So(rejection[types.MetricExhaustion].Raw, ShouldBeGreaterThan, 0)

			So(recoiled[types.MetricCompression].Raw, ShouldBeGreaterThan, 0)
			So(recoiled[types.MetricExhaustion].Raw, ShouldEqual, 0)
			So(reignition[types.MetricRVOL].Raw,
				ShouldBeGreaterThan, ignition[types.MetricRVOL].Raw)
			So(reignition[types.MetricIgnition].Raw,
				ShouldBeGreaterThan, ignition[types.MetricIgnition].Raw)
			So(ignition[types.MetricStrength].Raw,
				ShouldBeGreaterThan, baseline[types.MetricStrength].Raw)
			So(rejection[types.MetricStrength].Raw, ShouldBeGreaterThan, 0)
			So(reignition[types.MetricStrength].Raw,
				ShouldBeGreaterThan, ignition[types.MetricStrength].Raw)
		})
	})
}

/*
TestSignal_MeasureFromMarketRequiresJointEvidence compares a real ignition leg
with twins that carry only its volume or price component.
*/
func TestSignal_MeasureFromMarketRequiresJointEvidence(t *testing.T) {
	Convey("Given equally calibrated market paths", t, func() {
		measureFinal := func(
			priceMultiplier float64,
			tradeQuantity float64,
			spread float64,
			depth float64,
		) map[types.MetricType]*types.Measurement {
			prices := []float64{100, 100.1, 100.2001, 100.3003001, 100.4006004}
			quantities := []float64{10, 10, 10, 10, 10}
			spreads := []float64{0.2, 0.2, 0.2, 0.2, 0.2}
			depths := []float64{1_000, 1_000, 1_000, 1_000, 1_000}
			prices = append(prices, prices[len(prices)-1]*priceMultiplier)
			quantities = append(quantities, tradeQuantity)
			spreads = append(spreads, spread)
			depths = append(depths, depth)
			epochs := measureMarket(
				t,
				conditions.MarketPath(prices, quantities, spreads, depths).Frames(),
			)

			return indexEpoch(epochs[len(epochs)-1])
		}

		volumeOnly := measureFinal(1, 200, 0.2, 1_000)
		priceOnly := measureFinal(1.10, 0, 0.2, 1_000)
		thinBook := measureFinal(1.10, 1, 10, 1)
		joint := measureFinal(1.10, 200, 0.2, 1_000)

		Convey("Then neither isolated component is confused with joint ignition", func() {
			So(volumeOnly[types.MetricRVOL].Raw,
				ShouldBeGreaterThan, priceOnly[types.MetricRVOL].Raw)
			So(volumeOnly[types.MetricPrecursor].Raw, ShouldEqual, 0)
			So(volumeOnly[types.MetricIgnition].Raw, ShouldEqual, 0)
			So(volumeOnly[types.MetricTrend].Raw, ShouldEqual, 0)
			So(priceOnly[types.MetricPrecursor].Raw, ShouldBeGreaterThan, 0)
			So(joint[types.MetricRVOL].Raw,
				ShouldBeGreaterThan, priceOnly[types.MetricRVOL].Raw)
			// On the volume clock, price displacement is measured per equal-volume
			// bar, so a volume-backed lift is a stronger precursor than the same
			// nominal move printed on little or no volume.
			So(joint[types.MetricPrecursor].Raw,
				ShouldBeGreaterThanOrEqualTo, priceOnly[types.MetricPrecursor].Raw)
			So(joint[types.MetricIgnition].Raw,
				ShouldBeGreaterThan, priceOnly[types.MetricIgnition].Raw)
			So(joint[types.MetricTrend].Raw,
				ShouldBeGreaterThan, priceOnly[types.MetricTrend].Raw)
			So(thinBook[types.MetricSpread].Raw,
				ShouldBeGreaterThan, joint[types.MetricSpread].Raw)
			So(thinBook[types.MetricRVOL].Raw,
				ShouldBeLessThan, joint[types.MetricRVOL].Raw)
			So(thinBook[types.MetricIgnition].Raw,
				ShouldBeLessThan, joint[types.MetricIgnition].Raw)
			So(thinBook[types.MetricStrength].Raw,
				ShouldBeLessThan, joint[types.MetricStrength].Raw)
		})
	})
}
