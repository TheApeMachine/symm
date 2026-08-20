package sentiment

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"

	"github.com/theapemachine/symm/kraken"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

func TestSentimentNumber(t *testing.T) {
	Convey("Given ticks on one symbol", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		market := thesis.Symbol("AAA/USD")
		start := time.Unix(1_700_000_000, 0).UTC()

		for index, price := range []float64{100, 102, 101, 105} {
			market.AppendTicker(kraken.TickerData{
				Symbol:    "AAA/USD",
				Last:      decimal.NewFromFloat64(price),
				Timestamp: start.Add(time.Duration(index) * time.Second),
			})
		}

		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()
		go signal.Run()

		Convey("It should emit a sentiment deviation reading per tick", func() {
			measurements := []*nmtypes.Measurement{}

			time.Sleep(50 * time.Millisecond)
			for measurement := range market.MarketMeasurements("category") {
				measurements = append(measurements, measurement)
			}

			So(len(measurements), ShouldEqual, 4)
			So(measurements[0].Source, ShouldEqual, string(types.SourceSentiment))
		})
	})

	Convey("Given pending ticker observations in regressing queue order", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		market := thesis.Symbol("AAA/USD")
		start := time.Unix(1_700_000_000, 0).UTC()

		market.AppendTicker(kraken.TickerData{
			Symbol:    "AAA/USD",
			Timestamp: start.Add(time.Second),
		})
		market.AppendTicker(kraken.TickerData{
			Symbol:    "AAA/USD",
			Timestamp: start,
		})

		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()
		err := signal.step([]*types.Symbol{market})

		Convey("It should process every observation in event-time order", func() {
			measurements := make([]*nmtypes.Measurement, 0, 2)

			for measurement := range market.MarketMeasurements("category") {
				measurements = append(measurements, measurement)
			}

			So(err, ShouldBeNil)
			So(len(measurements), ShouldEqual, 2)
			So(measurements[0].At.Before(measurements[1].At), ShouldBeTrue)
		})
	})

	Convey("Given an older observation arriving after a completed cohort step", t, func() {
		thesis := types.NewThesis(context.Background(), nil)
		market := thesis.Symbol("AAA/USD")
		start := time.Unix(1_700_000_000, 0).UTC()
		signal := NewSignal(context.Background(), thesis)
		defer signal.Close()

		market.AppendTicker(kraken.TickerData{Symbol: "AAA/USD", Timestamp: start.Add(time.Second)})
		err := signal.step([]*types.Symbol{market})
		So(err, ShouldBeNil)

		market.AppendTicker(kraken.TickerData{Symbol: "AAA/USD", Timestamp: start})
		err = signal.step([]*types.Symbol{market})

		Convey("It should retain the late observation without regressing the cohort clock", func() {
			measurements := make([]*nmtypes.Measurement, 0, 2)

			for measurement := range market.MarketMeasurements("category") {
				measurements = append(measurements, measurement)
			}

			So(err, ShouldBeNil)
			So(len(measurements), ShouldEqual, 2)
			So(measurements[1].At.Equal(start), ShouldBeTrue)
		})
	})
}

func BenchmarkSentimentNumber(b *testing.B) {
	const cohortSize = 200

	start := time.Unix(1_700_000_000, 0).UTC()
	b.ReportAllocs()

	for range b.N {
		thesis := types.NewThesis(context.Background(), nil)
		symbols := make([]*types.Symbol, 0, cohortSize)

		for symbolIndex := range cohortSize {
			symbol := thesis.Symbol(strconv.Itoa(symbolIndex) + "/USD")
			symbols = append(symbols, symbol)
			symbol.AppendTicker(kraken.TickerData{
				Symbol:    symbol.Symbol,
				Timestamp: start.Add(time.Duration(cohortSize-symbolIndex) * time.Nanosecond),
			})
		}

		signal := NewSignal(context.Background(), thesis)

		if err := signal.step(symbols); err != nil {
			b.Fatal(err)
		}
	}
}
