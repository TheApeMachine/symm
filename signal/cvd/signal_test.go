package cvd

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/types"
)

func measureField(measurements []*types.Measurement, symbol string, metric types.MetricType) (*types.Measurement, bool) {
	for index := len(measurements) - 1; index >= 0; index-- {
		measurement := measurements[index]

		if measurement.Symbol == symbol && measurement.Metric == metric {
			return measurement, true
		}
	}

	return nil, false
}

func measurementFields(measurements []*types.Measurement, symbol string) map[types.MetricType]float64 {
	return tests.MeasurementFields(measurements, symbol)
}

func newSignal() *Signal {
	return &Signal{
		ctx:    context.Background(),
		sample: algorithm.NewTradeFlowSample(),
		flow:   equation.NewFlow(),
	}
}

func frameFor(rows ...kraken.TradeData) *types.MarketFrame {
	return &types.MarketFrame{
		Trades:       rows,
		CrossSection: types.NewCrossSection(),
	}
}

func TestSignal_Measure(testingTB *testing.T) {
	Convey("Given a CVD signal", testingTB, func() {
		signal := newSignal()

		Convey("When measuring repeated buy trades with rising price", func() {
			var measurements []*types.Measurement

			for _, row := range trades("MATIC/USD", "buy", 100, 1, 30, time.Now().UTC()) {
				result, err := signal.Calculate(frameFor(row))
				So(err, ShouldBeNil)
				measurements = result
			}

			Convey("Then CVD measurements should be emitted", func() {
				strength, ok := measureField(measurements, "MATIC/USD", types.MetricStrength)
				So(ok, ShouldBeTrue)
				So(strength.Raw, ShouldBeGreaterThan, 0)

				drive, ok := measureField(measurements, "MATIC/USD", types.MetricDrive)
				So(ok, ShouldBeTrue)
				So(drive.Raw, ShouldBeGreaterThan, 0)
			})
		})
	})
}

func TestSignal_MeasureFlowProfiles(testingTB *testing.T) {
	Convey("Given controlled trade-flow sequences", testingTB, func() {
		type flowCase struct {
			name      string
			rows      []kraken.TradeData
			wantScore string
		}

		start := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		cases := []flowCase{
			{
				name: "hidden absorption",
				rows: []kraken.TradeData{
					tradeRow("BTC/USD", "buy", 100, 10, start),
					tradeRow("BTC/USD", "buy", 100, 10, start.Add(time.Second)),
					tradeRow("BTC/USD", "buy", 100, 10, start.Add(2*time.Second)),
					tradeRow("BTC/USD", "buy", 100, 10, start.Add(3*time.Second)),
					tradeRow("BTC/USD", "buy", 100, 10, start.Add(4*time.Second)),
				},
				wantScore: "absorption",
			},
			{
				name: "aggressive drive",
				rows: []kraken.TradeData{
					tradeRow("BTC/USD", "buy", 100, 2, start),
					tradeRow("BTC/USD", "buy", 101, 2, start.Add(time.Second)),
					tradeRow("BTC/USD", "buy", 102, 2, start.Add(2*time.Second)),
					tradeRow("BTC/USD", "buy", 103, 2, start.Add(3*time.Second)),
					tradeRow("BTC/USD", "buy", 104, 2, start.Add(4*time.Second)),
				},
				wantScore: "drive",
			},
			{
				name: "stochastic balance",
				rows: []kraken.TradeData{
					tradeRow("BTC/USD", "buy", 100, 2, start),
					tradeRow("BTC/USD", "sell", 100, 2, start.Add(time.Second)),
					tradeRow("BTC/USD", "buy", 100.1, 2, start.Add(2*time.Second)),
					tradeRow("BTC/USD", "sell", 100.1, 2, start.Add(3*time.Second)),
				},
				wantScore: "balance",
			},
			{
				name: "volume starvation",
				rows: []kraken.TradeData{
					tradeRow("BTC/USD", "buy", 100, 2, start),
					tradeRow("BTC/USD", "sell", 100, 2, start.Add(time.Second)),
					tradeRow("BTC/USD", "buy", 100.01, 2, start.Add(2*time.Second)),
					tradeRow("BTC/USD", "sell", 100.01, 2, start.Add(3*time.Second)),
					tradeRow("BTC/USD", "buy", 100.02, 2, start.Add(4*time.Second)),
					tradeRow("BTC/USD", "sell", 100.02, 2, start.Add(5*time.Second)),
					tradeRow("BTC/USD", "buy", 100.03, 2, start.Add(6*time.Second)),
					tradeRow("BTC/USD", "sell", 100.03, 2, start.Add(7*time.Second)),
					tradeRow("BTC/USD", "buy", 100.04, 0.001, start.Add(8*time.Second)),
					tradeRow("BTC/USD", "sell", 100.04, 0.001, start.Add(9*time.Second)),
					tradeRow("BTC/USD", "buy", 100.04, 0.001, start.Add(10*time.Second)),
					tradeRow("BTC/USD", "sell", 100.04, 0.001, start.Add(11*time.Second)),
					tradeRow("BTC/USD", "buy", 100.04, 0.001, start.Add(12*time.Second)),
				},
				wantScore: "starvation",
			},
		}

		for _, testCase := range cases {
			testCase := testCase

			Convey(fmt.Sprintf("When measuring %s", testCase.name), func() {
				signal := newSignal()
				var metrics map[types.MetricType]float64

				wantMetric := map[string]types.MetricType{
					"absorption": types.MetricAbsorption,
					"drive":      types.MetricDrive,
					"balance":    types.MetricBalance,
					"starvation": types.MetricStarvation,
				}[testCase.wantScore]

				for _, row := range testCase.rows {
					result, err := signal.Calculate(frameFor(row))
					So(err, ShouldBeNil)

					symbolMetrics := measurementFields(result, "BTC/USD")

					if len(symbolMetrics) == 0 {
						continue
					}

					metrics = symbolMetrics
				}

				Convey(fmt.Sprintf("Then CVD should emphasize %s", testCase.wantScore), func() {
					So(metrics, ShouldNotBeNil)

					selected := metrics[wantMetric]

					for _, competitor := range []types.MetricType{
						types.MetricAbsorption, types.MetricDrive, types.MetricBalance, types.MetricStarvation,
					} {
						if competitor == wantMetric {
							continue
						}

						So(selected, ShouldBeGreaterThanOrEqualTo, metrics[competitor])
					}

					So(metrics[types.MetricStrength], ShouldBeGreaterThan, 0)
				})
			})
		}
	})
}

func sessionSignals(
	ctx context.Context,
	api *websocket.API,
	_ *broker.Instrument,
	channel chan []byte,
) []types.Signal {
	return []types.Signal{NewSignal(ctx, api, channel)}
}

func TestSignal_MeasureFromMarket(testingTB *testing.T) {
	Convey("Given CVD inside a paper Session market", testingTB, func() {
		calmSession, err := tests.NewSession(context.Background(), testingTB, tests.SessionOptions{
			Signals: sessionSignals,
		})
		So(err, ShouldBeNil)
		hotSession, err := tests.NewSession(context.Background(), testingTB, tests.SessionOptions{
			Signals: sessionSignals,
		})
		So(err, ShouldBeNil)

		Convey("When calm and aggression trade tapes play through Cut", func() {
			calmTheses, err := calmSession.Play(conditions.Calm(32).Frames())
			So(err, ShouldBeNil)
			hotTheses, err := hotSession.Play(
				conditions.Aggression(32, 8, 8).Frames(),
			)
			So(err, ShouldBeNil)

			calm, hasCalm := tests.PeakSourceMetric(
				calmTheses, types.SourceCVD, conditions.Subject(), types.MetricDrive,
			)
			hot, hasHot := tests.PeakSourceMetric(
				hotTheses, types.SourceCVD, conditions.Subject(), types.MetricDrive,
			)

			Convey("Then buy aggression lifts drive versus calm sells", func() {
				So(hasCalm, ShouldBeTrue)
				So(hasHot, ShouldBeTrue)
				So(hot, ShouldBeGreaterThan, calm)
			})
		})
	})
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	rows := trades("MATIC/USD", "buy", 100, 1, 8, time.Now().UTC())
	signal := newSignal()

	for _, row := range rows {
		_, _ = signal.Calculate(frameFor(row))
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, _ = signal.Calculate(frameFor(rows[len(rows)-1]))
	}
}

func trades(
	symbol string,
	side string,
	price float64,
	quantity float64,
	count int,
	start time.Time,
) []kraken.TradeData {
	rows := make([]kraken.TradeData, 0, count)

	for index := range count {
		rows = append(rows, tradeRow(
			symbol,
			side,
			price+float64(index)*0.01,
			quantity,
			start.Add(time.Duration(index)*time.Second),
		))
	}

	return rows
}

func tradeRow(
	symbol string,
	side string,
	price float64,
	quantity float64,
	at time.Time,
) kraken.TradeData {
	return kraken.TradeData{
		Symbol:    symbol,
		Side:      side,
		Price:     *decimal.NewFromFloat64(price),
		Qty:       quantity,
		Timestamp: at,
	}
}
