package hawkes

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
measureHawkesMarket drives precisely timed trade arrivals through the injected
Conn and production Market and returns the final Hawkes evidence epoch.
*/
func measureHawkesMarket(
	t testing.TB,
	frames iter.Seq[tests.Frame],
) []*types.Measurement {
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
	signal := NewSignal(ctx, api, nil)
	var final []*types.Measurement
	cutAt := time.Date(2024, 1, 1, 1, 0, 0, 0, time.UTC)

	for frame := range frames {
		mock.Emit(frame.Channel, frame.Payload)
		cut, cutErr := market.Cut(cutAt)
		So(cutErr, ShouldBeNil)
		cutAt = cutAt.Add(time.Second)

		if cut.IsEmpty() {
			continue
		}

		if types.SignalInterest(signal)&types.FrameInterest(cut) == 0 {
			continue
		}

		measurements := signal.Measure(types.NewThesis(nil, cut))

		if frame.Channel == "trade" && len(measurements) > 0 {
			final = measurements
		}
	}

	return final
}

/*
hawkesScenario creates an alternating or one-sided arrival stream whose price
and quantity may vary independently from its event times.
*/
func hawkesScenario(
	gap time.Duration,
	quantity float64,
	sides []string,
) *tests.Market {
	startedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	prices := make([]float64, len(sides))
	quantities := make([]float64, len(sides))
	stamps := make([]time.Time, len(sides))

	for index := range sides {
		prices[index] = 100 + float64(index)/100
		quantities[index] = quantity
		stamps[index] = startedAt.Add(time.Duration(index) * gap)
	}

	return conditions.TradePath(prices, quantities, sides, stamps)
}

/*
hawkesKey retains metric and directional identity when indexing an evidence
epoch containing several measurements with the same metric.
*/
func hawkesKey(metric types.MetricType, side types.MeasurementSide) string {
	return string(metric) + ":" + string(side)
}

/*
indexHawkesEpoch indexes one Hawkes bundle by metric and directional side.
*/
func indexHawkesEpoch(
	measurements []*types.Measurement,
) map[string]*types.Measurement {
	indexed := make(map[string]*types.Measurement, len(measurements))

	for _, measurement := range measurements {
		indexed[hawkesKey(measurement.Metric, measurement.Side)] = measurement
	}

	return indexed
}

/*
TestSignal_MeasureFromMarket proves empirical arrival evidence, fitted model
invariants, cadence sensitivity, mark invariance, and one-sided non-readiness.
*/
func TestSignal_MeasureFromMarket(t *testing.T) {
	Convey("Given balanced Hawkes streams with independently controlled cadence", t, func() {
		sides := make([]string, 20)

		for index := range sides {
			if index%2 == 0 {
				sides[index] = "buy"
				continue
			}

			sides[index] = "sell"
		}

		steady := indexHawkesEpoch(measureHawkesMarket(
			t, hawkesScenario(time.Second, 1, sides).Frames(),
		))
		burst := indexHawkesEpoch(measureHawkesMarket(
			t, hawkesScenario(100*time.Millisecond, 1, sides).Frames(),
		))
		largeMarks := indexHawkesEpoch(measureHawkesMarket(
			t, hawkesScenario(time.Second, 1_000, sides).Frames(),
		))
		oneSided := make([]string, 20)

		for index := range oneSided {
			oneSided[index] = "buy"
		}

		oneSide := indexHawkesEpoch(measureHawkesMarket(
			t, hawkesScenario(time.Second, 1, oneSided).Frames(),
		))

		Convey("Then the fitted balanced stream publishes every promised quantity", func() {
			expected := []struct {
				metric  types.MetricType
				side    types.MeasurementSide
				subject types.SubjectType
				unit    types.MeasurementUnit
			}{
				{types.MetricEventCount, types.SideNone, types.SubjectTradeArrivals, types.UnitCount},
				{types.MetricEventCount, types.SideBuy, types.SubjectTradeArrivals, types.UnitCount},
				{types.MetricEventCount, types.SideSell, types.SubjectTradeArrivals, types.UnitCount},
				{types.MetricArrivalRate, types.SideBuy, types.SubjectTradeArrivals, types.UnitEventsPerSecond},
				{types.MetricArrivalRate, types.SideSell, types.SubjectTradeArrivals, types.UnitEventsPerSecond},
				{types.MetricConditionalIntensity, types.SideBuy, types.SubjectHawkesProcess, types.UnitEventsPerSecond},
				{types.MetricConditionalIntensity, types.SideSell, types.SubjectHawkesProcess, types.UnitEventsPerSecond},
				{types.MetricBaselineIntensity, types.SideBuy, types.SubjectHawkesProcess, types.UnitEventsPerSecond},
				{types.MetricBaselineIntensity, types.SideSell, types.SubjectHawkesProcess, types.UnitEventsPerSecond},
				{types.MetricExcitationAmplitude, types.SideBuyToBuy, types.SubjectHawkesKernel, types.UnitEventsPerSecond},
				{types.MetricExcitationAmplitude, types.SideSellToBuy, types.SubjectHawkesKernel, types.UnitEventsPerSecond},
				{types.MetricExcitationAmplitude, types.SideBuyToSell, types.SubjectHawkesKernel, types.UnitEventsPerSecond},
				{types.MetricExcitationAmplitude, types.SideSellToSell, types.SubjectHawkesKernel, types.UnitEventsPerSecond},
				{types.MetricDecayRate, types.SideNone, types.SubjectHawkesKernel, types.UnitInverseSecond},
				{types.MetricKernelMemory, types.SideNone, types.SubjectHawkesKernel, types.UnitSecond},
				{types.MetricSpectralRadius, types.SideNone, types.SubjectHawkesProcess, types.UnitDimensionless},
				{types.MetricHawkesPoissonDelta, types.SideNone, types.SubjectHawkesFit, types.UnitNat},
				{types.MetricCrossSelfDelta, types.SideNone, types.SubjectHawkesFit, types.UnitNat},
				{types.MetricImmediateOffspring, types.SideBuy, types.SubjectHawkesProcess, types.UnitDimensionless},
				{types.MetricImmediateOffspring, types.SideSell, types.SubjectHawkesProcess, types.UnitDimensionless},
				{types.MetricTotalDescendants, types.SideBuy, types.SubjectHawkesProcess, types.UnitDimensionless},
				{types.MetricTotalDescendants, types.SideSell, types.SubjectHawkesProcess, types.UnitDimensionless},
			}
			So(len(steady), ShouldEqual, len(expected))

			for _, contract := range expected {
				measurement, found := steady[hawkesKey(contract.metric, contract.side)]
				So(found, ShouldBeTrue)
				So(measurement.Source, ShouldEqual, types.SourceHawkes)
				So(measurement.Stream, ShouldEqual, types.Hawkes)
				So(measurement.Symbol, ShouldEqual, conditions.Subject())
				So(measurement.Subject, ShouldEqual, contract.subject)
				So(measurement.Unit, ShouldEqual, contract.unit)
				So(math.IsNaN(measurement.Raw), ShouldBeFalse)
				So(math.IsInf(measurement.Raw, 0), ShouldBeFalse)
				So(measurement.ValidateStruct(), ShouldBeNil)
			}
		})

		Convey("Then counts, rates, model stability, and descendants are coherent", func() {
			So(steady[hawkesKey(types.MetricEventCount, types.SideNone)].Raw,
				ShouldEqual, 20)
			So(steady[hawkesKey(types.MetricEventCount, types.SideBuy)].Raw,
				ShouldEqual, 10)
			So(steady[hawkesKey(types.MetricEventCount, types.SideSell)].Raw,
				ShouldEqual, 10)
			So(burst[hawkesKey(types.MetricArrivalRate, types.SideBuy)].Raw,
				ShouldBeGreaterThan, steady[hawkesKey(types.MetricArrivalRate, types.SideBuy)].Raw)
			So(burst[hawkesKey(types.MetricArrivalRate, types.SideSell)].Raw,
				ShouldBeGreaterThan, steady[hawkesKey(types.MetricArrivalRate, types.SideSell)].Raw)
			spectral := steady[hawkesKey(types.MetricSpectralRadius, types.SideNone)].Raw
			So(spectral, ShouldBeGreaterThanOrEqualTo, 0)
			So(spectral, ShouldBeLessThan, 1)
			So(steady[hawkesKey(types.MetricBaselineIntensity, types.SideBuy)].Raw,
				ShouldBeGreaterThan, 0)
			So(steady[hawkesKey(types.MetricBaselineIntensity, types.SideSell)].Raw,
				ShouldBeGreaterThan, 0)
			So(steady[hawkesKey(types.MetricDecayRate, types.SideNone)].Raw,
				ShouldBeGreaterThan, 0)
			So(steady[hawkesKey(types.MetricKernelMemory, types.SideNone)].Raw,
				ShouldBeGreaterThan, 0)

			for _, side := range []types.MeasurementSide{types.SideBuy, types.SideSell} {
				immediate := steady[hawkesKey(types.MetricImmediateOffspring, side)].Raw
				total := steady[hawkesKey(types.MetricTotalDescendants, side)].Raw
				So(immediate, ShouldBeGreaterThanOrEqualTo, 0)
				So(total, ShouldBeGreaterThanOrEqualTo, immediate)
			}
		})

		Convey("Then price and quantity marks do not alter an arrival-only model", func() {
			So(len(largeMarks), ShouldEqual, len(steady))

			for key, steadyMeasurement := range steady {
				markedMeasurement, found := largeMarks[key]
				So(found, ShouldBeTrue)
				So(markedMeasurement.Raw, ShouldAlmostEqual, steadyMeasurement.Raw)
			}
		})

		Convey("Then a one-sided stream remains empirical rather than pretending to fit", func() {
			So(oneSide[hawkesKey(types.MetricEventCount, types.SideBuy)].Raw,
				ShouldEqual, 20)
			So(oneSide[hawkesKey(types.MetricEventCount, types.SideSell)].Raw,
				ShouldEqual, 0)
			So(oneSide[hawkesKey(types.MetricSpectralRadius, types.SideNone)], ShouldBeNil)
			So(oneSide[hawkesKey(types.MetricConditionalIntensity, types.SideBuy)], ShouldBeNil)
		})
	})
}
