package cvd

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestTradeOn(testingTB *testing.T) {
	Convey("Given a CVD trade ingestor", testingTB, func() {
		trade := &Trade{cache: []kraken.TradeData{}}
		payload := []byte(`{"channel":"trade","type":"update","data":[{"symbol":"BTC/USD","side":"buy","price":100.5,"qty":1.25,"ord_type":"market","trade_id":1,"timestamp":"2023-09-25T09:04:31.742648Z"}]}`)

		Convey("When a trade frame arrives", func() {
			trade.On(payload)

			Convey("Then trade rows should accumulate in cache", func() {
				So(len(trade.cache), ShouldEqual, 1)
				So(trade.cache[0].Symbol, ShouldEqual, "BTC/USD")
			})
		})
	})
}

func TestSignal_Measure(testingTB *testing.T) {
	Convey("Given a CVD signal", testingTB, func() {
		signal := &Signal{
			ctx:    context.Background(),
			trade:  &Trade{cache: []kraken.TradeData{}},
			sample: algorithm.NewTradeFlowSample(),
			flow:   equation.NewFlow(),
		}

		Convey("When measuring repeated buy trades with rising price", func() {
			var result *types.Thesis

			for _, row := range trades("MATIC/USD", "buy", 100, 1, 30, time.Now().UTC()) {
				signal.trade.cache = []kraken.TradeData{row}
				result = signal.Measure(types.NewThesis(nil))
			}

			Convey("Then CVD measurements should be emitted", func() {
				raw, ok := result.Measurements.Load("cvd")
				So(ok, ShouldBeTrue)

				metrics := raw.(datura.Map[datura.Map[*decimal.Decimal]])["MATIC/USD"]
				So(metrics, ShouldNotBeNil)
				So(metrics["strength"].Float64(), ShouldBeGreaterThan, 0)
				So(metrics["drive"].Float64(), ShouldBeGreaterThan, 0)
				So(len(signal.trade.cache), ShouldEqual, 0)
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
				signal := &Signal{
					ctx:    context.Background(),
					trade:  &Trade{cache: []kraken.TradeData{}},
					sample: algorithm.NewTradeFlowSample(),
					flow:   equation.NewFlow(),
				}
				var metrics datura.Map[*decimal.Decimal]

				for _, row := range testCase.rows {
					signal.trade.cache = []kraken.TradeData{row}
					result := signal.Measure(types.NewThesis(nil))

					raw, ok := result.Measurements.Load("cvd")

					if !ok {
						continue
					}

					symbolMetrics := raw.(datura.Map[datura.Map[*decimal.Decimal]])["BTC/USD"]

					if symbolMetrics == nil {
						continue
					}

					metrics = symbolMetrics
				}

				Convey(fmt.Sprintf("Then CVD should emphasize %s", testCase.wantScore), func() {
					So(metrics, ShouldNotBeNil)

					selected := metrics[testCase.wantScore].Float64()
					for key, value := range metrics {
						if key == "net" || key == "netFraction" || key == "category" || key == "maturity" || key == "strength" {
							continue
						}

						if key == testCase.wantScore {
							continue
						}

						So(selected, ShouldBeGreaterThanOrEqualTo, value.Float64())
					}

					So(metrics["strength"].Float64(), ShouldBeGreaterThan, 0)
				})
			})
		}
	})
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	rows := trades("MATIC/USD", "buy", 100, 1, 8, time.Now().UTC())
	signal := &Signal{
		ctx:    context.Background(),
		trade:  &Trade{cache: []kraken.TradeData{}},
		sample: algorithm.NewTradeFlowSample(),
		flow:   equation.NewFlow(),
	}

	for _, row := range rows {
		signal.trade.cache = []kraken.TradeData{row}
		_ = signal.Measure(types.NewThesis(nil))
	}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		signal.trade.cache = append([]kraken.TradeData(nil), rows[len(rows)-1])
		_ = signal.Measure(types.NewThesis(nil))
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
