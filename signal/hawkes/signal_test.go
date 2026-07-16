package hawkes

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm/excitation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func newTestSignal() *Signal {
	return &Signal{
		ctx:      context.Background(),
		sample:   excitation.NewSample(),
		process:  excitation.NewProcess(),
		evidence: NewEvidence(),
	}
}

func frameOf(rows ...kraken.TradeData) *types.MarketFrame {
	return &types.MarketFrame{Trades: rows, CrossSection: types.NewCrossSection()}
}

func measureField(
	measurements []*types.Measurement, symbol string, metric types.MetricType,
) (*types.Measurement, bool) {
	for index := len(measurements) - 1; index >= 0; index-- {
		measurement := measurements[index]

		if measurement.Symbol == symbol && measurement.Metric == metric {
			return measurement, true
		}
	}

	return nil, false
}

func tradeRow(symbol, side string, price float64, quantity float64, at time.Time) kraken.TradeData {
	return kraken.TradeData{
		Symbol:    symbol,
		Side:      side,
		Price:     *decimal.NewFromFloat64(price),
		Qty:       quantity,
		Timestamp: at,
	}
}

func TestSignalOnTrade(testingTB *testing.T) {
	Convey("Given a Hawkes signal driven by the central market cut", testingTB, func() {
		signal := newTestSignal()
		at := time.Date(2023, 9, 25, 9, 4, 31, 0, time.UTC)
		row := tradeRow("BTC/USD", "buy", 100.5, 1.25, at)

		Convey("When a trade frame is calculated", func() {
			_, err := signal.Calculate(frameOf(row))

			Convey("Then Calculate should accept the row without error", func() {
				So(err, ShouldBeNil)
			})
		})

		Convey("When an empty frame arrives", func() {
			measurements, err := signal.Calculate(frameOf())

			Convey("Then nothing should be measured", func() {
				So(err, ShouldBeNil)
				So(measurements, ShouldBeEmpty)
			})
		})

		Convey("When frame calculations overlap measurement drains", func() {
			wait := sync.WaitGroup{}
			wait.Add(2)

			go func() {
				defer wait.Done()

				for range 100 {
					signal.Calculate(frameOf(row))
				}
			}()

			go func() {
				defer wait.Done()

				for range 100 {
					<-signal.Measure(types.NewThesis(nil, frameOf(row)))
				}
			}()

			wait.Wait()
		})
	})
}

func TestSignalMeasure(testingTB *testing.T) {
	Convey("Given a Hawkes signal with enough marked arrivals to identify a stream", testingTB, func() {
		signal := newTestSignal()
		start := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		sides := []string{"buy", "sell", "buy", "sell", "buy", "sell"}
		var result []*types.Measurement

		Convey("When each trade arrives on its own tick", func() {
			for index, side := range sides {
				frame := frameOf(
					tradeRow("BTC/USD", side, 100+float64(index), 1, start.Add(time.Duration(index)*time.Second)),
				)
				measurements, err := signal.Calculate(frame)
				So(err, ShouldBeNil)
				result = measurements
			}

			Convey("Then event-count observation measurements should be emitted", func() {
				count, ok := measureField(result, "BTC/USD", types.MetricEventCount)
				So(ok, ShouldBeTrue)
				So(count.Raw, ShouldBeGreaterThan, 0)
			})
		})
	})
}
