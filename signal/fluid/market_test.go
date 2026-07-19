package fluid

import (
	"context"
	"iter"
	"testing"
	"time"

	krakendecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/tests/mockapi"
	"github.com/theapemachine/symm/trader"
	"github.com/theapemachine/symm/types"
)

func measureMarket(t testing.TB, frames iter.Seq[tests.Frame]) []*types.Measurement {
	t.Helper()
	withFluidInterval(t)
	pinFluidViper(t, map[string]any{
		"signals.feed_timeline_capacity": 128,
		"signals.feed_track_capacity":    128,
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
	signal := NewSignal(ctx, api, instrument, nil)
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

		measurements = append(
			measurements,
			signal.Measure(types.NewThesis(nil, cut))...,
		)
	}

	return measurements
}

/*
fluidActivityEpoch returns the coherent subject bundle at the path's greatest
Reynolds observation without mixing other metric peaks from different moments.
*/
func fluidActivityEpoch(
	measurements []*types.Measurement,
	symbol string,
) map[types.MetricType]*types.Measurement {
	var at time.Time
	peak := 0.0
	found := false

	for _, measurement := range measurements {

		if measurement.Source == types.SourceFluid &&
			measurement.Symbol == symbol && measurement.Metric == types.MetricReynolds {
			if !found || measurement.Raw > peak {
				peak = measurement.Raw
				at = measurement.At
				found = true
			}
		}
	}

	epoch := make(map[types.MetricType]*types.Measurement)

	for _, measurement := range measurements {
		if measurement.Source == types.SourceFluid &&
			measurement.Symbol == symbol && measurement.At.Equal(at) {
			epoch[measurement.Metric] = measurement
		}
	}

	return epoch
}

/*
TestSignal_MeasureFromMarket proves fluid publishes its complete mechanical
state for stable, churning, and depleted production market paths.
*/
func TestSignal_MeasureFromMarket(t *testing.T) {
	Convey("Given stable, churning, and depth-depleted markets", t, func() {
		symbol := conditions.Subject()
		stableMeasurements := measureMarket(t, conditions.TapeCalm().Frames())
		churningMeasurements := measureMarket(t, conditions.MarketPath(
			[]float64{0.5667, 0.5668, 0.5667, 0.5668, 0.5667, 0.5668, 0.5667, 0.5668, 0.5667, 0.5668, 0.5667, 0.5668},
			[]float64{50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50, 50},
			[]float64{0.0002, 0.0002, 0.0002, 0.0002, 0.0002, 0.0002, 0.0002, 0.0002, 0.0002, 0.0002, 0.0002, 0.0002},
			[]float64{100, 20, 140, 10, 160, 30, 120, 15, 180, 25, 130, 10},
		).Frames())
		depletedMeasurements := measureMarket(t, conditions.TapeExhaustion().Frames())
		stable := fluidActivityEpoch(stableMeasurements, symbol)
		churning := fluidActivityEpoch(churningMeasurements, symbol)
		depleted := fluidActivityEpoch(depletedMeasurements, symbol)
		metrics := []types.MetricType{
			types.MetricLaminarScore,
			types.MetricTurbulentScore,
			types.MetricInertialScore,
			types.MetricViscousScore,
			types.MetricViscosity,
			types.MetricReynolds,
			types.MetricDivergenceV2,
			types.MetricVelocityCurvatureV2,
			types.MetricTurbulence,
			types.MetricSourceBalance,
			types.MetricMemory,
			types.MetricMidAddRate,
			types.MetricMidExecuteRate,
		}

		Convey("Then every regime emits the complete valid metric contract", func() {
			for _, epoch := range []map[types.MetricType]*types.Measurement{
				stable, churning, depleted,
			} {
				for _, metric := range metrics {
					measurement := epoch[metric]
					So(measurement, ShouldNotBeNil)
					So(measurement.ValidateStruct(), ShouldBeNil)
				}
			}
		})

		Convey("Then the raw mechanics respond to churn and depletion", func() {
			So(stable[types.MetricLaminarScore].Raw, ShouldAlmostEqual, 1, 1e-12)
			So(stable[types.MetricReynolds].Raw, ShouldEqual, 0)
			So(churning[types.MetricReynolds].Raw, ShouldBeGreaterThan, 0)
			So(churning[types.MetricTurbulentScore].Raw, ShouldBeGreaterThan, stable[types.MetricTurbulentScore].Raw)
			So(churning[types.MetricLaminarScore].Raw, ShouldBeLessThan, stable[types.MetricLaminarScore].Raw)
			So(churning[types.MetricMemory].Raw, ShouldBeGreaterThan, 0)
			So(depleted[types.MetricReynolds].Raw, ShouldBeGreaterThan, churning[types.MetricReynolds].Raw)
		})
	})
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	withFluidInterval(benchmark)
	signal := NewSignal(context.Background(), nil, nil, nil)
	at := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	fixture := &symbolBookFixture{symbol: "BTC/USD"}

	for index := 0; index < 4; index++ {
		stamp := at.Add(time.Duration(index) * 200 * time.Millisecond)
		ticker := kraken.TickerData{
			Symbol:    "BTC/USD",
			Bid:       krakendecimal.NewFromFloat64(99.5),
			Ask:       krakendecimal.NewFromFloat64(100.5),
			Last:      krakendecimal.NewFromFloat64(100),
			Volume:    1_000,
			Timestamp: stamp,
		}
		if _, err := signal.Calculate(&types.MarketFrame{
			Tickers:      []kraken.TickerData{ticker},
			CrossSection: types.NewCrossSection(),
		}); err != nil {
			benchmark.Fatal(err)
		}
		row := fixture.snapshot(99.5, 20-float64(index), 100.5, 10)
		row.Timestamp = stamp
		row.PriceIncrement = krakendecimal.NewFromFloat64(0.01)
		if _, err := signal.Calculate(&types.MarketFrame{
			Tickers:      []kraken.TickerData{ticker},
			Books:        []kraken.BookData{row},
			CrossSection: types.NewCrossSection(),
		}); err != nil {
			benchmark.Fatal(err)
		}
	}

	stamp := at.Add(time.Second)
	ticker := kraken.TickerData{
		Symbol:    "BTC/USD",
		Bid:       krakendecimal.NewFromFloat64(99.5),
		Ask:       krakendecimal.NewFromFloat64(100.5),
		Last:      krakendecimal.NewFromFloat64(100),
		Volume:    1_200,
		Timestamp: stamp,
	}
	row := fixture.snapshot(99.5, 12, 100.5, 14)
	row.Timestamp = stamp
	row.PriceIncrement = krakendecimal.NewFromFloat64(0.01)
	frame := &types.MarketFrame{
		Tickers:      []kraken.TickerData{ticker},
		Books:        []kraken.BookData{row},
		CrossSection: types.NewCrossSection(),
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		if _, err := signal.Calculate(frame); err != nil {
			benchmark.Fatal(err)
		}
	}
}
