package toxicity

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

func TestSignal_MeasureUsesTradeEventTime(testingTB *testing.T) {
	Convey("Given cached trades with explicit event timestamps", testingTB, func() {
		eventAt := time.Unix(42, 500_000_000).UTC()
		signal := &Signal{
			ctx:    context.Background(),
			trades: &Trade{cache: tradeCache()},
			level3: &Level3{
				api: websocket.NewAPI(context.Background(), nil, nil, nil),
			},
		}
		signal.trades.On([]byte(`{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","side":"buy","price":100,"qty":1,"ord_type":"market","trade_id":1,"timestamp":"1970-01-01T00:00:42.5Z"}]}`))

		result := signal.Measure(types.NewThesis(nil))

		Convey("Then measurements use the trade event time and typed subjects", func() {
			So(result.Measurements, ShouldNotBeEmpty)

			volume := result.Measurements[0]
			So(volume.At, ShouldEqual, eventAt)
			So(volume.Subject, ShouldEqual, types.SubjectLevel3Tape)
			So(volume.Maturity, ShouldBeGreaterThan, 0)
			So(volume.Validity.State, ShouldEqual, types.ValidityProvisional)
			So(volume.Validity.Readiness, ShouldEqual, types.ReadinessObservation)
			So(volume.Normalized, ShouldBeNil)
			So(volume.Scale.Kind, ShouldEqual, types.ScaleObservationWindow)
		})

		Convey("Then websocket writes can overlap measurement drains", func() {
			payload := []byte(`{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","side":"buy","price":100,"qty":1,"ord_type":"market","trade_id":1,"timestamp":"1970-01-01T00:00:42.5Z"}]}`)
			wait := sync.WaitGroup{}
			wait.Add(2)

			go func() {
				defer wait.Done()

				for range 100 {
					signal.trades.On(payload)
				}
			}()

			go func() {
				defer wait.Done()

				for range 100 {
					signal.Measure(types.NewThesis(nil))
				}
			}()

			wait.Wait()
		})
	})
}

func TestSignal_MeasureSkipsTradeWithoutTimestamp(testingTB *testing.T) {
	Convey("Given a trade row without event time", testingTB, func() {
		signal := &Signal{
			ctx: context.Background(),
			trades: &Trade{
				cache: tradeCache(
					kraken.TradeData{
						Symbol: "BTC/USD",
						Price:  *decimal.NewFromFloat64(100),
						Qty:    1,
					},
				),
			},
			level3: &Level3{
				api: websocket.NewAPI(context.Background(), nil, nil, nil),
			},
		}

		result := signal.Measure(types.NewThesis(nil))

		Convey("Then it emits nothing instead of inventing wall-clock time", func() {
			So(result.Measurements, ShouldBeEmpty)
		})
	})
}

func TestSignal_touchHonestyEmitsCancelledQuantity(testingTB *testing.T) {
	Convey("Given a prior touch snapshot and a retreating bid", testingTB, func() {
		eventAt := time.Unix(50, 0).UTC()
		signal := &Signal{}
		row := &symbolEvidence{
			latestAt:   eventAt,
			tradeCount: 1,
		}
		prior := touchSnapshot{
			bidQuantity: 5,
			askQuantity: 3,
			observedAt:  time.Unix(49, 0),
		}

		measurements := signal.touchHonesty(
			"BTC/USD", row, prior, 2, 3, 0.5,
		)

		Convey("Then it emits cancelled and retreating touch evidence", func() {
			cancelled, ok := latestMetric(measurements, types.MetricCancelledQuantity, types.SideBuy)
			So(ok, ShouldBeTrue)
			So(cancelled.Raw, ShouldEqual, 3)
			So(cancelled.Validity.State, ShouldEqual, types.ValidityProvisional)
			So(cancelled.Normalized, ShouldNotBeNil)
			So(*cancelled.Normalized, ShouldAlmostEqual, 0.6, 1e-9)

			retreating, ok := latestMetric(measurements, types.MetricRetreatingQuantity, types.SideBuy)
			So(ok, ShouldBeTrue)
			So(retreating.Raw, ShouldEqual, 3)
			So(retreating.At, ShouldEqual, eventAt)
		})
	})
}

func latestMetric(
	measurements []*types.Measurement,
	metric types.MetricType,
	side types.MeasurementSide,
) (*types.Measurement, bool) {
	for index := len(measurements) - 1; index >= 0; index-- {
		measurement := measurements[index]

		if measurement.Metric == metric && measurement.Side == side {
			return measurement, true
		}
	}

	return nil, false
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	benchmark.ReportAllocs()

	eventAt := time.Unix(1, 0).UTC()
	signal := &Signal{
		ctx: context.Background(),
		trades: &Trade{
			cache: tradeCache(
				kraken.TradeData{
					Symbol:    "BTC/USD",
					Timestamp: eventAt,
					Price:     *decimal.NewFromFloat64(100),
					Qty:       1,
				},
			),
		},
		level3: &Level3{
			api: websocket.NewAPI(context.Background(), nil, nil, nil),
		},
	}

	for benchmark.Loop() {
		signal.trades.cache = tradeCache(
			kraken.TradeData{
				Symbol:    "BTC/USD",
				Timestamp: eventAt,
				Price:     *decimal.NewFromFloat64(100),
				Qty:       1,
			},
		)
		signal.Measure(types.NewThesis(nil))
	}
}
