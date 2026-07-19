package correlation

import (
	"context"
	"iter"
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

func measureMarket(t testing.TB, frames iter.Seq[tests.Frame]) []*types.Measurement {
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
	measurements := make([]*types.Measurement, 0)
	cutAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

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

		measurements = append(
			measurements,
			signal.Measure(types.NewThesis(nil, cut))...,
		)
	}

	return measurements
}

/*
lastCorrelationEpoch returns the final complete subject bundle so assertions do
not combine metric peaks from different moments in the path.
*/
func lastCorrelationEpoch(
	measurements []*types.Measurement,
	symbol string,
) map[types.MetricType]*types.Measurement {
	var at time.Time

	for index := len(measurements) - 1; index >= 0; index-- {
		measurement := measurements[index]

		if measurement.Source == types.SourceCorrelation &&
			measurement.Symbol == symbol && measurement.Metric == types.MetricStrength {
			at = measurement.At
			break
		}
	}

	epoch := make(map[types.MetricType]*types.Measurement)

	for _, measurement := range measurements {
		if measurement.Source == types.SourceCorrelation &&
			measurement.Symbol == symbol && measurement.At.Equal(at) {
			epoch[measurement.Metric] = measurement
		}
	}

	return epoch
}

/*
TestSignal_MeasureFromMarket proves correlation separates aligned, excess,
opposed, and unstructured cohort paths through the production market feed.
*/
func TestSignal_MeasureFromMarket(t *testing.T) {
	Convey("Given independently defined cohort relationship regimes", t, func() {
		symbol := conditions.Subject()
		herd := lastCorrelationEpoch(
			measureMarket(t, conditions.TapeSectorLift().Frames()), symbol,
		)
		alpha := lastCorrelationEpoch(
			measureMarket(t, conditions.TapeAlpha().Frames()), symbol,
		)
		stress := lastCorrelationEpoch(
			measureMarket(t, conditions.TapeDivergence().Frames()), symbol,
		)
		noise := lastCorrelationEpoch(
			measureMarket(t, conditions.TapeNoise().Frames()), symbol,
		)
		metrics := []types.MetricType{
			types.MetricCorrelation,
			types.MetricSigned,
			types.MetricRelativeEnergy,
			types.MetricHerdScore,
			types.MetricAlphaScore,
			types.MetricNoiseScore,
			types.MetricStressScore,
			types.MetricPeakScore,
			types.MetricStrength,
		}

		Convey("Then every regime emits the complete valid metric contract", func() {
			for _, epoch := range []map[types.MetricType]*types.Measurement{
				herd, alpha, stress, noise,
			} {
				So(epoch, ShouldHaveLength, len(metrics))

				for _, metric := range metrics {
					measurement := epoch[metric]
					So(measurement, ShouldNotBeNil)
					So(measurement.Source, ShouldEqual, types.SourceCorrelation)
					So(measurement.Validity.State, ShouldEqual, types.ValidityValid)
					So(measurement.ValidateStruct(), ShouldBeNil)
				}
			}
		})

		Convey("Then an equal-energy aligned subject is systemic herd motion", func() {
			So(herd[types.MetricCorrelation].Raw, ShouldBeGreaterThan, 0.9)
			So(herd[types.MetricSigned].Raw, ShouldBeGreaterThan, 0.9)
			So(herd[types.MetricRelativeEnergy].Raw, ShouldAlmostEqual, 1, 0.1)
			So(herd[types.MetricHerdScore].Raw, ShouldBeGreaterThan, 0.8)
			So(herd[types.MetricHerdScore].Raw, ShouldBeGreaterThan, herd[types.MetricAlphaScore].Raw)
			So(herd[types.MetricHerdScore].Raw, ShouldBeGreaterThan, herd[types.MetricNoiseScore].Raw)
			So(herd[types.MetricHerdScore].Raw, ShouldBeGreaterThan, herd[types.MetricStressScore].Raw)
		})

		Convey("Then aligned excess return energy is alpha rather than herd", func() {
			So(alpha[types.MetricSigned].Raw, ShouldBeGreaterThan, 0.9)
			So(alpha[types.MetricRelativeEnergy].Raw, ShouldBeGreaterThan, 2)
			So(alpha[types.MetricAlphaScore].Raw, ShouldBeGreaterThan, alpha[types.MetricHerdScore].Raw)
			So(alpha[types.MetricAlphaScore].Raw, ShouldBeGreaterThan, alpha[types.MetricNoiseScore].Raw)
		})

		Convey("Then a rising subject against falling peers is divergent stress", func() {
			So(stress[types.MetricSigned].Raw, ShouldBeLessThan, -0.9)
			So(stress[types.MetricStressScore].Raw, ShouldBeGreaterThan, 0.9)
			So(stress[types.MetricStressScore].Raw, ShouldBeGreaterThan, stress[types.MetricHerdScore].Raw)
			So(stress[types.MetricStressScore].Raw, ShouldBeGreaterThan, stress[types.MetricAlphaScore].Raw)
		})

		Convey("Then an unstructured path retains noise evidence", func() {
			So(noise[types.MetricCorrelation].Raw, ShouldBeLessThan, herd[types.MetricCorrelation].Raw)
			So(noise[types.MetricNoiseScore].Raw, ShouldBeGreaterThan, 0)
		})

		Convey("Then peak and strength report the dominant score exactly", func() {
			for _, epoch := range []map[types.MetricType]*types.Measurement{
				herd, alpha, stress, noise,
			} {
				dominant := max(
					max(epoch[types.MetricHerdScore].Raw, epoch[types.MetricAlphaScore].Raw),
					max(epoch[types.MetricNoiseScore].Raw, epoch[types.MetricStressScore].Raw),
				)
				So(epoch[types.MetricPeakScore].Raw, ShouldAlmostEqual, dominant, 1e-12)
				So(epoch[types.MetricStrength].Raw, ShouldAlmostEqual, dominant, 1e-12)
			}
		})
	})
}
