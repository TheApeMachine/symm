package correlation

import (
	"context"
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	nomcorrelation "github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

var correlationCategories = []logic.CategoryType{
	logic.CategorySystemicHerd,
	logic.CategoryDecoupledAlpha,
	logic.CategoryStochasticNoise,
	logic.CategoryDivergentStress,
}

func TestSignalMeasureCategorySemantics(testingTB *testing.T) {
	Convey("Given correlated cross-section returns", testingTB, func() {
		crossSection := testCrossSection(testingTB)
		signal := NewSignal(context.Background())
		defer signal.Close()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD"}
		var result *logic.Measurement

		for tick := range 8 {
			at := base.Add(time.Duration(tick) * 10 * time.Second)
			changePct := 0.5 + float64(tick)*0.1

			for symbolIndex, symbol := range symbols {
				last := (100 + float64(symbolIndex)) * math.Pow(1.2, float64(tick))
				input := tickerInput(at, symbol, last, changePct)
				So(crossSection.Observe(input.Ticker), ShouldBeNil)

				measured, err := lastMeasurement(signal, crossSection, input)
				So(err, ShouldBeNil)

				if measured != nil {
					result = measured
				}
			}
		}

		Convey("It should emit non-uniform cohort classification", func() {
			So(result, ShouldNotBeNil)
			So(result.Confidence, ShouldBeGreaterThan, 0.25)
			So(result.Metric("peakScore"), ShouldBeGreaterThan, 0)
			So(distributionSum(result, correlationCategories), ShouldAlmostEqual, 1, 0.0001)
			So(result.DominantCategory(), ShouldNotEqual, logic.CategoryTypeNone)
		})
	})

	Convey("Given a decoupled high-energy symbol", testingTB, func() {
		crossSection := testCrossSection(testingTB)
		signal := NewSignal(context.Background())
		defer signal.Close()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		peers := []string{"BTC/USD", "ETH/USD", "SOL/USD", "ADA/USD"}
		peerReturns := map[string][]float64{
			"BTC/USD": {0.012, -0.010, 0.011, -0.009, 0.012, -0.010, 0.011, -0.009},
			"ETH/USD": {0.011, -0.009, 0.012, -0.010, 0.011, -0.009, 0.012, -0.010},
			"SOL/USD": {0.013, -0.011, 0.010, -0.008, 0.013, -0.011, 0.010, -0.008},
			"ADA/USD": {0.010, -0.008, 0.011, -0.009, 0.010, -0.008, 0.011, -0.009},
		}
		alphaReturns := []float64{0.180, 0.160, -0.140, -0.120, 0.170, 0.150, -0.130, -0.110}
		peerLast := map[string]float64{
			"BTC/USD": 100,
			"ETH/USD": 100,
			"SOL/USD": 100,
			"ADA/USD": 100,
		}
		alphaLast := 50.0
		var result *logic.Measurement

		for tick := range 24 {
			at := base.Add(time.Duration(tick) * time.Minute)
			cycle := tick % len(peerReturns["BTC/USD"])
			alphaCycle := tick % len(alphaReturns)

			for _, symbol := range peers {
				returnRate := peerReturns[symbol][cycle]
				peerLast[symbol] *= 1 + returnRate
				input := tickerInput(at, symbol, peerLast[symbol], returnRate*100)
				So(crossSection.Observe(input.Ticker), ShouldBeNil)
				_, err := lastMeasurement(signal, crossSection, input)
				So(err, ShouldBeNil)
			}

			alphaReturn := alphaReturns[alphaCycle]
			alphaLast *= 1 + alphaReturn
			input := tickerInput(at, "ALPHA/USD", alphaLast, alphaReturn*100)
			So(crossSection.Observe(input.Ticker), ShouldBeNil)

			measured, err := lastMeasurement(signal, crossSection, input)
			So(err, ShouldBeNil)

			if measured != nil {
				result = measured
			}
		}

		Convey("It should classify decoupled alpha", func() {
			So(result, ShouldNotBeNil)
			So(result.DominantCategory(), ShouldEqual, logic.CategoryDecoupledAlpha)
			So(result.Metric("alphaScore"), ShouldBeGreaterThan, 0)
			So(result.Metric("alphaScore"), ShouldBeGreaterThan, result.Metric("herdScore"))
			So(result.Metric("alphaScore"), ShouldBeGreaterThan, result.Metric("stressScore"))
			So(result.Confidence, ShouldBeGreaterThan, 0.25)
		})
	})

	Convey("Given asynchronous returns where ticks do not align perfectly", testingTB, func() {
		crossSection := testCrossSection(testingTB)
		signal := NewSignal(context.Background())
		defer signal.Close()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		var result *logic.Measurement

		for index := 1; index <= 10; index++ {
			atA := base.Add(time.Duration(2*index-1) * time.Second)
			inputA := tickerInput(atA, "ASYNC_A/USD", 10+float64(index)*0.5, 5)
			So(crossSection.Observe(inputA.Ticker), ShouldBeNil)
			_, err := lastMeasurement(signal, crossSection, inputA)
			So(err, ShouldBeNil)

			atB := base.Add(time.Duration(2*index) * time.Second)
			inputB := tickerInput(atB, "ASYNC_B/USD", 20+float64(index), 5)
			So(crossSection.Observe(inputB.Ticker), ShouldBeNil)

			measured, err := lastMeasurement(signal, crossSection, inputB)
			So(err, ShouldBeNil)

			if measured != nil {
				result = measured
			}
		}

		Convey("It should use cross-section timestamped samples and report high correlation", func() {
			So(result, ShouldNotBeNil)
			So(result.Metric("correlation"), ShouldBeGreaterThan, 0.8)
			So(result.Metric("peakScore"), ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalCorrelation(testingTB *testing.T) {
	Convey("Given asynchronous proportional price samples", testingTB, func() {
		signal := NewSignal(context.Background())
		defer signal.Close()

		start := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		left := []nomcorrelation.Sample{
			{At: start, Value: 100},
			{At: start.Add(time.Second), Value: 110},
			{At: start.Add(3 * time.Second), Value: 121},
		}
		right := []nomcorrelation.Sample{
			{At: start.Add(500 * time.Millisecond), Value: 50},
			{At: start.Add(1500 * time.Millisecond), Value: 55},
			{At: start.Add(2500 * time.Millisecond), Value: 60.5},
		}

		Convey("When Signal.correlation evaluates the pair", func() {
			value, ok := signal.correlation(left, right)

			Convey("It should use Hayashi-Yoshida overlap correlation", func() {
				So(ok, ShouldBeTrue)
				So(value, ShouldAlmostEqual, 1, 1e-9)
			})
		})
	})
}

func TestSignalMeasureUsesEveryTickerRow(testingTB *testing.T) {
	Convey("Given multi-row ticker input", testingTB, func() {
		crossSection := testCrossSection(testingTB)
		signal := NewSignal(context.Background())
		defer signal.Close()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		lasts := map[string]float64{
			"BTC/USD": 100,
			"ETH/USD": 80,
			"SOL/USD": 40,
		}
		returns := map[string][]float64{
			"BTC/USD": {0.010, -0.004, 0.013, 0.002, 0.009, -0.003, 0.011, 0.004},
			"ETH/USD": {0.008, -0.003, 0.011, 0.003, 0.007, -0.002, 0.010, 0.003},
			"SOL/USD": {0.020, 0.006, -0.012, 0.018, -0.007, 0.016, -0.010, 0.014},
		}
		var thirdRowMeasured *logic.Measurement

		for tick := range 8 {
			for symbol, series := range returns {
				lasts[symbol] *= 1 + series[tick]
			}

			input := tickerInput(
				base.Add(time.Duration(tick)*time.Second),
				"BTC/USD", lasts["BTC/USD"], 1,
				"ETH/USD", lasts["ETH/USD"], 1,
				"SOL/USD", lasts["SOL/USD"], 1,
			)
			So(crossSection.Observe(input.Ticker), ShouldBeNil)

			measurements, err := signal.Measure(input, crossSection)
			So(err, ShouldBeNil)

			for _, measurement := range measurements {
				if measurement.Symbol == "SOL/USD" {
					thirdRowMeasured = measurement
				}
			}
		}

		Convey("It should measure rows beyond data[0]", func() {
			So(thirdRowMeasured, ShouldNotBeNil)
			So(thirdRowMeasured.Confidence, ShouldBeGreaterThan, 0)
		})
	})
}

func BenchmarkSignalMeasure(benchmark *testing.B) {
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	symbols := []string{"BTC/USD", "ETH/USD", "SOL/USD"}

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		crossSection := testCrossSection(benchmark)
		signal := NewSignal(context.Background())

		for tick := range 8 {
			at := base.Add(time.Duration(tick) * 10 * time.Second)

			for symbolIndex, symbol := range symbols {
				input := tickerInput(at, symbol, 100+float64(tick)+float64(symbolIndex), 0.5)
				_ = crossSection.Observe(input.Ticker)
				_, _ = lastMeasurement(signal, crossSection, input)
			}
		}

		_ = signal.Close()
	}
}

func testCrossSection(testingTB testing.TB) *market.CrossSection {
	if testingTB != nil {
		testingTB.Helper()
	}

	crossSection, err := market.NewCrossSection(
		market.CrossSectionConfig{
			ReturnCap:  16,
			MinBars:    6,
			BreadthCap: 16,
		},
	)

	if err != nil && testingTB != nil {
		testingTB.Fatal(err)
	}

	return crossSection
}

func tickerInput(at time.Time, values ...any) market.Input {
	rows := make(kraken.TickerDataSlice, 0, len(values)/3)

	for index := 0; index < len(values); index += 3 {
		rows = append(rows, kraken.TickerData{
			Symbol:    values[index].(string),
			Last:      testNumber(values[index+1]),
			Volume:    1000,
			ChangePct: testNumber(values[index+2]),
			Timestamp: at,
		})
	}

	return market.Input{
		Role:   "ticker",
		At:     at,
		Ticker: rows,
	}
}

func testNumber(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	default:
		panic("ticker fixture value must be numeric")
	}
}

func lastMeasurement(
	signal *Signal,
	crossSection *market.CrossSection,
	input market.Input,
) (*logic.Measurement, error) {
	measurements, err := signal.Measure(input, crossSection)
	if err != nil {
		return nil, err
	}

	if len(measurements) == 0 {
		return nil, nil
	}

	return measurements[len(measurements)-1], nil
}

func distributionSum(
	measurement *logic.Measurement,
	categories []logic.CategoryType,
) float64 {
	total := 0.0

	for _, category := range categories {
		total += measurement.Distribution[category]
	}

	return total
}
