package fluid

import (
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestBookMeasureWaitsForMarketState(t *testing.T) {
	Convey("Given a book without ticker-fed volume", t, withFluidGrid(nil, func() {
		registry := NewSyncRegistry()
		book := NewBook(registry)
		eventAt := time.Date(2026, 7, 10, 4, 0, 0, 0, time.UTC)

		Convey("When a snapshot arrives before market state is ready", func() {
			measurements, err := book.Measure(kraken.BookData{
				Symbol: "BTC/USD",
				Type:   "snapshot",
				Bids: []kraken.BookLevel{
					{Price: *decimal.NewFromFloat64(99), Qty: 5},
				},
				Asks: []kraken.BookLevel{
					{Price: *decimal.NewFromFloat64(101), Qty: 5},
				},
				Timestamp: eventAt,
			})

			Convey("Then it should wait instead of erroring", func() {
				So(err, ShouldBeNil)
				So(measurements, ShouldBeNil)
			})
		})

		Convey("When a scaled mechanical reading is emitted", func() {
			observedFrom := eventAt.Add(-time.Second)
			reading := fluidReading{
				symbol:            "BTC/USD",
				reynolds:          0.5,
				divergence:        0.1,
				viscosity:         645450.5,
				velocityCurvature: 0.2,
				turbulence:        0.3,
				sourceBalance:     2,
				memory:            0.4,
				midAddRate:        3,
				midExecuteRate:    4,
				gridSteps:         2,
				dynamics: fluidDynamics{
					stamps:                   []float64{float64(observedFrom.UnixNano()), float64(eventAt.UnixNano())},
					reynoldsHistory:          []float64{0.5, 0.5},
					divergenceHistory:        []float64{0.1, 0.1},
					viscosityHistory:         []float64{645450.5, 645450.5},
					velocityCurvatureHistory: []float64{0.2, 0.2},
					turbulenceHistory:        []float64{0.3, 0.3},
				},
			}

			measurements, err := book.measurementsFromReading(reading, eventAt)
			So(err, ShouldBeNil)
			seen := make(map[types.MetricType]*types.Measurement, len(measurements))

			for _, measurement := range measurements {
				_, duplicate := seen[measurement.Metric]
				So(duplicate, ShouldBeFalse)
				seen[measurement.Metric] = measurement
			}

			Convey("Then it should publish unique category-free evidence with real scale", func() {
				So(measurements, ShouldHaveLength, 13)
				So(seen[types.MetricStrength], ShouldBeNil)
				So(seen[types.MetricType("laminar")], ShouldBeNil)
				So(seen[types.MetricType("turbulent")], ShouldBeNil)
				So(seen[types.MetricType("inertial")], ShouldBeNil)
				So(seen[types.MetricType("viscous")], ShouldBeNil)
				So(seen[types.MetricLaminarScore].Raw, ShouldEqual, 1)
				So(seen[types.MetricLaminarScore].Normalized, ShouldBeNil)
				So(seen[types.MetricLaminarScore].ObservedFrom, ShouldResemble, observedFrom)
				So(seen[types.MetricViscosity].Raw, ShouldEqual, 645450.5)
				So(seen[types.MetricViscosity].ObservedFrom, ShouldResemble, eventAt)
				So(seen[types.MetricDivergenceV2].Unit, ShouldEqual, types.UnitInverseSecond)
				So(seen[types.MetricMidAddRate].Unit, ShouldEqual, types.UnitBaseCurrencyPerSecond)
			})
		})
	}))
}

func TestBookMeasureOutOfOrderHistoryInterval(t *testing.T) {
	Convey("Given a fluid reading whose stamp trail is not chronological", t, func() {
		eventAt := time.Date(2026, 7, 10, 4, 0, 0, 0, time.UTC)
		observedFrom := eventAt.Add(-2 * time.Second)
		reading := fluidReading{
			symbol:            "BTC/USD",
			reynolds:          0.5,
			divergence:        0.1,
			viscosity:         645450.5,
			velocityCurvature: 0.2,
			turbulence:        0.3,
			sourceBalance:     2,
			memory:            0.4,
			midAddRate:        3,
			midExecuteRate:    4,
			gridSteps:         2,
			dynamics: fluidDynamics{
				stamps: []float64{
					float64(eventAt.Add(time.Second).UnixNano()),
					float64(observedFrom.UnixNano()),
					float64(eventAt.UnixNano()),
				},
				reynoldsHistory:          []float64{0.5, 0.5, 0.5},
				divergenceHistory:        []float64{0.1, 0.1, 0.1},
				viscosityHistory:         []float64{645450.5, 645450.5, 645450.5},
				velocityCurvatureHistory: []float64{0.2, 0.2, 0.2},
				turbulenceHistory:        []float64{0.3, 0.3, 0.3},
			},
		}
		book := NewBook(NewSyncRegistry())

		Convey("When history-backed measurements are emitted", func() {
			measurements, err := book.measurementsFromReading(reading, eventAt)

			Convey("Then every evidence interval should run forward", func() {
				So(err, ShouldBeNil)

				for _, measurement := range measurements {
					So(measurement.ValidateStruct(), ShouldBeNil)
				}
			})
		})
	})
}

func TestBookMeasureSkipsEmptyLevels(t *testing.T) {
	Convey("Given a book update with no levels on either side", t, withFluidGrid(nil, func() {
		registry := NewSyncRegistry()
		book := NewBook(registry)
		eventAt := time.Date(2026, 7, 10, 4, 0, 0, 0, time.UTC)

		Convey("When the frame is a checksum-only refresh or a thin market", func() {
			measurements, err := book.Measure(kraken.BookData{
				Symbol:    "BTC/USD",
				Type:      "update",
				Timestamp: eventAt,
			})

			Convey("Then it should skip without erroring", func() {
				So(err, ShouldBeNil)
				So(measurements, ShouldBeNil)
			})
		})

		Convey("When a real snapshot has already been recorded", func() {
			_, snapshotErr := book.Measure(kraken.BookData{
				Symbol: "BTC/USD",
				Type:   "snapshot",
				Bids: []kraken.BookLevel{
					{Price: *decimal.NewFromFloat64(99), Qty: 5},
				},
				Asks: []kraken.BookLevel{
					{Price: *decimal.NewFromFloat64(101), Qty: 5},
				},
				Timestamp: eventAt,
			})
			So(snapshotErr, ShouldBeNil)

			state, stateErr := registry.loadSymbol("BTC/USD")
			So(stateErr, ShouldBeNil)
			bidsBefore := len(state.book.Bids)
			asksBefore := len(state.book.Asks)

			Convey("Then a subsequent empty-levels update should not erase the known book", func() {
				_, updateErr := book.Measure(kraken.BookData{
					Symbol:    "BTC/USD",
					Type:      "update",
					Timestamp: eventAt.Add(time.Second),
				})

				So(updateErr, ShouldBeNil)
				So(len(state.book.Bids), ShouldEqual, bidsBefore)
				So(len(state.book.Asks), ShouldEqual, asksBefore)
			})
		})
	}))
}
