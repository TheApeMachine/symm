package causal

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

func testResonanceManifold(
	energy, surprise float64,
	curve []float64,
) *learning.ResonanceManifold {
	coder := learning.NewResonanceManifold([]int{1, 2, 1}, 1, 0.1)
	prediction := 0.0

	if len(curve) > 0 {
		prediction = curve[0]
	}

	for index := range 8 {
		input := []float64{energy + surprise + float64(index+1)/10}
		_, _ = coder.SettleFromBatchOptions(input, []float64{prediction}, true, true)
	}

	return coder
}

func setCausalPrice(
	price *broker.Price,
	symbol string,
	midpoint float64,
	at time.Time,
) {
	price.Update(&kraken.TickerData{
		Symbol:    symbol,
		Bid:       decimal.NewFromFloat64(midpoint),
		Ask:       decimal.NewFromFloat64(midpoint),
		Last:      decimal.NewFromFloat64(midpoint),
		Timestamp: at,
	})
}

func causalSymbol(thesis *types.Thesis, symbol string) *types.Symbol {
	stored, _ := thesis.Symbols.LoadOrStore(symbol, types.NewSymbol(symbol))

	return stored.(*types.Symbol)
}

func lastCausal(symbolState *types.Symbol) (map[string]any, bool) {
	var output map[string]any

	for stored := range symbolState.MarketCausal(
		symbolState.CausalConsumers[types.CausalConsumerCausal],
	) {
		output = stored
	}

	return output, output != nil
}

func TestUpdate(t *testing.T) {
	convey.Convey("Given a predictive-coding reading without a forecast", t, func() {
		price := broker.NewPrice(websocket.NewAPI(t.Context(), nil, nil))
		thesis := types.NewThesis(t.Context(), nil)
		symbolState := causalSymbol(thesis, "BTC/USD")
		symbolState.Resonance.Push(learning.NewResonanceManifold([]int{1, 2, 1}, 1, 0.1))
		solver := NewSolver(types.NewThesis(t.Context(), nil), price, nil, nil)
		convey.Reset(func() {
			convey.So(solver.Close(), convey.ShouldBeNil)
		})

		err := solver.measure(thesis, "BTC/USD")

		convey.Convey("Then causal should complete without inventing an estimate", func() {
			convey.So(err, convey.ShouldBeNil)
			_, found := lastCausal(symbolState)
			convey.So(found, convey.ShouldBeFalse)
		})
	})

	convey.Convey("Given a forecast followed by a later executable midpoint", t, func() {
		price := broker.NewPrice(websocket.NewAPI(t.Context(), nil, nil))
		thesis := types.NewThesis(t.Context(), nil)
		symbol := "BTC/USD"
		firstAt := time.Unix(1, 0)
		symbolState := causalSymbol(thesis, symbol)
		symbolState.Resonance.Push(testResonanceManifold(0.5, 0.25, []float64{0.1}))
		setCausalPrice(price, symbol, 100, firstAt)
		solver := NewSolver(types.NewThesis(t.Context(), nil), price, nil, nil)
		convey.Reset(func() {
			convey.So(solver.Close(), convey.ShouldBeNil)
		})

		convey.So(solver.measure(thesis, symbol), convey.ShouldBeNil)
		_, found := lastCausal(symbolState)
		convey.So(found, convey.ShouldBeFalse)

		thesis.At = thesis.At.Add(time.Second)
		symbolState.Resonance.Push(testResonanceManifold(0.75, 0.5, []float64{0.2}))
		setCausalPrice(price, symbol, 110, firstAt.Add(time.Second))
		err := solver.measure(thesis, symbol)

		convey.Convey("Then it should retain the strictly prior unresolved row", func() {
			convey.So(err, convey.ShouldBeNil)
			output, found := lastCausal(symbolState)
			convey.So(found, convey.ShouldBeTrue)
			rows := output["historyRows"].([][]float64)
			convey.So(output["samples"], convey.ShouldEqual, 1)
			convey.So(output["precision"], convey.ShouldEqual, 0.0)
			convey.So(output["treatmentLevel"], convey.ShouldNotBeNil)
			convey.So(output["identification"], convey.ShouldEqual, "unresolved")
			convey.So(rows, convey.ShouldHaveLength, 1)
			convey.So(rows[0], convey.ShouldHaveLength, 4)
			convey.So(math.IsNaN(rows[0][0]), convey.ShouldBeFalse)
			convey.So(math.IsNaN(rows[0][1]), convey.ShouldBeFalse)
			convey.So(math.IsNaN(rows[0][2]), convey.ShouldBeFalse)
			convey.So(rows[0][3], convey.ShouldAlmostEqual, math.Log(110.0/100.0), 1e-12)
			_, hasAssociation := output["association"]
			convey.So(hasAssociation, convey.ShouldBeFalse)
		})
	})

	convey.Convey("Given an invalid causal row configuration", t, func() {
		price := broker.NewPrice(websocket.NewAPI(t.Context(), nil, nil))
		thesis := types.NewThesis(t.Context(), nil)
		symbol := "BTC/USD"
		symbolState := causalSymbol(thesis, symbol)
		baseAt := time.Unix(1, 0)
		solver := NewSolver(
			makeEmptyThesis(),
			price,
			nil,
			nil,
			WithPearlConfig(algorithm.PearlConfig{
				Target:    99,
				Treatment: 2,
				Controls:  []int{0, 1},
			}),
		)
		convey.Reset(func() {
			convey.So(solver.Close(), convey.ShouldBeNil)
		})

		for index := range 4 {
			thesis.At = baseAt.Add(time.Duration(index) * time.Second)
			symbolState.Resonance.Push(testResonanceManifold(
				float64(index+1)/10,
				float64(index+2)/10,
				[]float64{float64(index+3) / 10},
			))
			setCausalPrice(price, symbol, 100+float64(index), thesis.At)

			if index < 3 {
				_ = solver.measure(thesis, symbol)
			}
		}

		err := solver.measure(thesis, symbol)

		convey.Convey("Then it should not hide real bad configuration as unresolved evidence", func() {
			convey.So(err, convey.ShouldNotBeNil)
			_, found := lastCausal(symbolState)
			convey.So(found, convey.ShouldBeFalse)
		})
	})

	convey.Convey("Given a causal evidence stream for one symbol", t, func() {
		price := broker.NewPrice(websocket.NewAPI(t.Context(), nil, nil))
		thesis := types.NewThesis(t.Context(), nil)
		symbol := "BTC/USD"
		symbolState := causalSymbol(thesis, symbol)
		baseAt := time.Unix(1, 0)
		midpoint := 100.0
		previousEnergy := 0.0
		previousSurprise := 0.0
		previousPrediction := 0.0
		solver := NewSolver(makeEmptyThesis(), price, nil, nil)
		convey.Reset(func() {
			convey.So(solver.Close(), convey.ShouldBeNil)
		})

		convey.Convey("It should retain forward rows and report finite-sample precision", func() {
			for index := range 13 {
				if index > 0 {
					realizedReturn := 0.5*previousEnergy +
						0.25*previousSurprise + 2*previousPrediction
					midpoint *= math.Exp(realizedReturn)
				}

				energy := float64(index%3) / 1_000
				surprise := float64((index*2)%5) / 1_000
				prediction := float64(index+1) / 1_000
				thesis.At = baseAt.Add(time.Duration(index) * time.Second)
				symbolState.Resonance.Push(testResonanceManifold(energy, surprise, []float64{prediction}))
				setCausalPrice(price, symbol, midpoint, thesis.At)

				err := solver.measure(thesis, symbol)
				convey.So(err, convey.ShouldBeNil)
				previousEnergy = energy
				previousSurprise = surprise
				previousPrediction = prediction
			}

			output, found := lastCausal(symbolState)
			convey.So(found, convey.ShouldBeTrue)

			convey.So(output["association"], convey.ShouldNotBeNil)
			convey.So(output["samples"], convey.ShouldEqual, 12)
			convey.So(output["precision"], convey.ShouldBeGreaterThan, 0.0)
			convey.So(output["precision"], convey.ShouldBeLessThan, 1.0)
			rows, rowsOK := output["historyRows"].([][]float64)
			convey.So(rowsOK, convey.ShouldBeTrue)
			convey.So(rows, convey.ShouldHaveLength, 12)
		})
	})
}

/*
makeEmptyThesis returns a thesis without symbols so a solver's self-running goroutine
has nothing to consume; the tests drive measure(thesis, symbol) synchronously on an
explicit thesis.
*/
func makeEmptyThesis() *types.Thesis {
	return types.NewThesis(context.Background(), nil)
}

func BenchmarkCausalRun(b *testing.B) {
	thesis := types.NewThesis(b.Context(), nil)
	solver := NewSolver(thesis, nil, nil, nil)
	b.Cleanup(func() {
		if err := solver.Close(); err != nil {
			b.Fatal(err)
		}
	})
	baseAt := time.Unix(1, 0)
	symbols := make([]string, 640)

	for index := range symbols {
		symbol := fmt.Sprintf("SYMBOL-%03d/USD", index)
		symbols[index] = symbol
		storedSymbol := causalSymbol(thesis, symbol)
		storedSymbol.Resonance.Push(testResonanceManifold(
			float64(index),
			float64(index)/float64(index+1),
			[]float64{float64(index) / float64(index+1)},
		))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		b.StopTimer()

		for index, symbol := range symbols {
			storedSymbol := causalSymbol(thesis, symbol)
			storedSymbol.Resonance.Push(testResonanceManifold(
				float64(index),
				float64(index)/float64(index+1),
				[]float64{float64(index) / float64(index+1)},
			))
		}

		b.StartTimer()

		for _, symbol := range symbols {
			storedSymbol := causalSymbol(thesis, symbol)
			storedSymbol.MarketResonance(
				storedSymbol.ResonanceConsumers[types.ResonanceConsumerCausal],
			)
		}

		thesis.At = baseAt.Add(1 * time.Second)
	}
}
