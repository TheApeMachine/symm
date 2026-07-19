package cvd

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
measureTradeMarket drives one explicit trade path through the injectable Conn
and production Market and returns its final CVD measurement epoch.
*/
func measureTradeMarket(
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

		measurements := signal.Measure(types.NewThesis(nil, cut))

		if frame.Channel == "trade" && len(measurements) > 0 {
			final = measurements
		}
	}

	return final
}

/*
tradeScenario builds an explicit single-symbol trade condition without
deriving aggressor side from the price path.
*/
func tradeScenario(
	prices []float64,
	quantities []float64,
	sides []string,
) *tests.Market {
	startedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	stamps := make([]time.Time, len(prices))

	for index := range stamps {
		stamps[index] = startedAt.Add(time.Duration(index) * time.Second)
	}

	return conditions.TradePath(prices, quantities, sides, stamps)
}

/*
indexMarketEpoch indexes the final CVD measurement bundle by its unique metric.
*/
func indexMarketEpoch(
	measurements []*types.Measurement,
) map[types.MetricType]*types.Measurement {
	indexed := make(map[types.MetricType]*types.Measurement, len(measurements))

	for _, measurement := range measurements {
		indexed[measurement.Metric] = measurement
	}

	return indexed
}

/*
TestSignal_MeasureFromMarket proves the four CVD regimes and an adverse-flow
twin from executed trades delivered through the production market path.
*/
func TestSignal_MeasureFromMarket(t *testing.T) {
	Convey("Given independently specified executed-trade regimes", t, func() {
		absorption := indexMarketEpoch(measureTradeMarket(t, tradeScenario(
			[]float64{100, 100, 100, 100, 100},
			[]float64{10, 10, 10, 10, 10},
			[]string{"buy", "buy", "buy", "buy", "buy"},
		).Frames()))
		drive := indexMarketEpoch(measureTradeMarket(t, tradeScenario(
			[]float64{100, 101, 102, 103, 104},
			[]float64{2, 2, 2, 2, 2},
			[]string{"buy", "buy", "buy", "buy", "buy"},
		).Frames()))
		balance := indexMarketEpoch(measureTradeMarket(t, tradeScenario(
			[]float64{100, 100, 100.1, 100.1},
			[]float64{2, 2, 2, 2},
			[]string{"buy", "sell", "buy", "sell"},
		).Frames()))
		starvation := indexMarketEpoch(measureTradeMarket(t, tradeScenario(
			[]float64{100, 100, 100.01, 100.01, 100.02, 100.02, 100.03, 100.03,
				100.04, 100.04, 100.04, 100.04, 100.04},
			[]float64{2, 2, 2, 2, 2, 2, 2, 2, 0.001, 0.001, 0.001, 0.001, 0.001},
			[]string{"buy", "sell", "buy", "sell", "buy", "sell", "buy", "sell",
				"buy", "sell", "buy", "sell", "buy"},
		).Frames()))
		adverse := indexMarketEpoch(measureTradeMarket(t, tradeScenario(
			[]float64{100, 101, 102, 103, 104},
			[]float64{2, 2, 2, 2, 2},
			[]string{"sell", "sell", "sell", "sell", "sell"},
		).Frames()))
		regimes := []map[types.MetricType]*types.Measurement{
			absorption, drive, balance, starvation, adverse,
		}
		expected := map[types.MetricType]types.MeasurementUnit{
			types.MetricAbsorption:  types.UnitDimensionless,
			types.MetricDrive:       types.UnitDimensionless,
			types.MetricBalance:     types.UnitDimensionless,
			types.MetricStarvation:  types.UnitDimensionless,
			types.MetricStrength:    types.UnitDimensionless,
			types.MetricNetFraction: types.UnitDimensionless,
			types.MetricNet:         types.UnitQuoteCurrency,
		}

		Convey("Then every regime publishes the complete CVD contract", func() {
			for _, regime := range regimes {
				So(len(regime), ShouldEqual, len(expected))

				for metric, unit := range expected {
					measurement, found := regime[metric]
					So(found, ShouldBeTrue)
					So(measurement.Source, ShouldEqual, types.SourceCVD)
					So(measurement.Stream, ShouldEqual, types.CVD)
					So(measurement.Subject, ShouldEqual, types.SubjectAggressorFlow)
					So(measurement.Symbol, ShouldEqual, conditions.Subject())
					So(measurement.Unit, ShouldEqual, unit)
					So(math.IsNaN(measurement.Raw), ShouldBeFalse)
					So(math.IsInf(measurement.Raw, 0), ShouldBeFalse)
					So(measurement.Validity.State, ShouldEqual, types.ValidityValid)
					So(measurement.ValidateStruct(), ShouldBeNil)
				}
			}
		})

		Convey("Then the regimes remain behaviorally distinct", func() {
			So(absorption[types.MetricAbsorption].Raw,
				ShouldBeGreaterThan, absorption[types.MetricDrive].Raw)
			So(absorption[types.MetricNet].Raw, ShouldBeGreaterThan, 0)
			So(drive[types.MetricDrive].Raw,
				ShouldBeGreaterThan, drive[types.MetricAbsorption].Raw)
			So(drive[types.MetricNet].Raw, ShouldBeGreaterThan, 0)
			So(balance[types.MetricBalance].Raw,
				ShouldBeGreaterThan, balance[types.MetricDrive].Raw)
			So(balance[types.MetricBalance].Raw,
				ShouldBeGreaterThan, balance[types.MetricAbsorption].Raw)
			So(starvation[types.MetricStarvation].Raw, ShouldBeGreaterThan, 0)
			So(adverse[types.MetricNet].Raw, ShouldBeLessThan, 0)
			So(adverse[types.MetricDrive].Raw, ShouldEqual, 0)
			So(adverse[types.MetricAbsorption].Raw, ShouldBeGreaterThan, 0)

			for _, regime := range regimes {
				So(regime[types.MetricNetFraction].Raw, ShouldBeGreaterThanOrEqualTo, 0)
				So(regime[types.MetricNetFraction].Raw, ShouldBeLessThanOrEqualTo, 1)
				So(regime[types.MetricStrength].Raw, ShouldBeGreaterThan, 0)
			}
		})
	})
}
