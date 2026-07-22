package signal

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestSignal_Bootstrap proves the historical seed print does not silently skew
the resting book that every later semantic scenario uses as its baseline.
*/
func TestSignal_Bootstrap(t *testing.T) {
	Convey("Given symmetric seed liquidity for several symbols", t, func() {
		symbols := []string{"SIM1/USD", "SIM2/USD", "SIM3/USD"}
		signal := New(symbols)
		initial := make(map[string]Quote, len(symbols))

		for _, symbol := range symbols {
			initial[symbol], _ = signal.Quote(symbol)
		}

		signal.Bootstrap()

		Convey("It should emit real alternating prints and restore their consumed touch", func() {
			for _, symbol := range symbols {
				quote, exists := signal.Quote(symbol)
				So(exists, ShouldBeTrue)
				So(quote, ShouldResemble, initial[symbol])
			}

			for samples := range signal.Generate() {
				for _, sample := range samples {
					So(sample.Traded, ShouldBeTrue)
					So(sample.Fills, ShouldNotBeEmpty)
				}
			}
		})
	})
}

/*
TestSignal_Transition proves semantic states produce deterministic price paths.
*/
func TestSignal_Transition(t *testing.T) {
	Convey("Given identical deterministic market signals", t, func() {
		left := New([]string{"SIM1/USD"})
		right := New([]string{"SIM1/USD"})
		leftPrices := []float64{}
		rightPrices := []float64{}
		So(left.Transition(FastPump), ShouldBeNil)
		So(right.Transition(FastPump), ShouldBeNil)

		for samples := range left.Generate() {
			leftPrices = append(leftPrices, samples[0].Price)
		}

		for samples := range right.Generate() {
			rightPrices = append(rightPrices, samples[0].Price)
		}

		Convey("When both transition into a fast pump", func() {
			So(leftPrices, ShouldResemble, rightPrices)
			So(leftPrices, ShouldHaveLength, fastLegObservations+settleObservations)
			So(leftPrices[len(leftPrices)-1], ShouldBeGreaterThan, leftPrices[0])
			So(leftPrices[fastLegObservations-1], ShouldAlmostEqual, initialPrice*(1+eventMoveFraction), 0.1)
			So(leftPrices[len(leftPrices)-1], ShouldAlmostEqual, initialPrice*(1+eventMoveFraction), 0.1)
		})

		Convey("When one transitions into a fast dump", func() {
			prices := []float64{}
			So(left.Transition(FastDump), ShouldBeNil)

			for samples := range left.Generate() {
				prices = append(prices, samples[0].Price)
			}

			So(prices[len(prices)-1], ShouldBeLessThan, prices[0])
		})
	})

	Convey("Given an idle market with several symbols", t, func() {
		signal := New([]string{"SIM1/USD", "SIM2/USD", "SIM3/USD"})
		So(signal.Transition(Baseline), ShouldBeNil)
		prices := map[string][]float64{}

		for samples := range signal.Generate() {
			for _, sample := range samples {
				prices[sample.Symbol] = append(prices[sample.Symbol], sample.Price)
			}
		}

		Convey("It should gently oscillate every symbol without changing its level", func() {
			So(prices, ShouldHaveLength, 3)

			for _, path := range prices {
				So(path, ShouldHaveLength, idleObservations)
				So(path[0], ShouldNotEqual, path[1])
				So(path[0], ShouldAlmostEqual, initialPrice, initialPrice*idleAmplitudeFraction*2)
				So(path[len(path)-1], ShouldAlmostEqual, initialPrice, initialPrice*idleAmplitudeFraction*2)
			}
		})
	})

	Convey("Given causal twins on one selected symbol", t, func() {
		symbols := []string{"SIM1/USD", "SIM2/USD"}
		absorption := New(symbols)
		lowVolumeLift := New(symbols)
		compression := New(symbols)
		initialQuote, exists := compression.Quote(symbols[0])
		So(exists, ShouldBeTrue)
		So(absorption.Transition(VolumeAbsorption, symbols[0]), ShouldBeNil)
		So(lowVolumeLift.Transition(LowVolumeLift, symbols[0]), ShouldBeNil)
		So(compression.Transition(SpreadCompression, symbols[0]), ShouldBeNil)
		_, err := absorption.Scenario(FastPump, "UNKNOWN/USD")
		So(err, ShouldNotBeNil)

		var absorbed []Sample
		var lifted []Sample

		for samples := range absorption.Generate() {
			absorbed = samples
		}

		for samples := range lowVolumeLift.Generate() {
			lifted = samples
		}

		compressedQuote, exists := compression.Quote(symbols[0])
		So(exists, ShouldBeTrue)

		Convey("It should vary one economic cause without moving the controls", func() {
			So(absorbed[0].Price, ShouldAlmostEqual, initialPrice, initialPrice*idleAmplitudeFraction*2)
			So(lifted[0].Price, ShouldBeGreaterThan, initialPrice)
			So(absorbed[0].Statistics.Volume, ShouldBeGreaterThan, lifted[0].Statistics.Volume)
			So(absorbed[1].Price, ShouldAlmostEqual, initialPrice, initialPrice*idleAmplitudeFraction*2)
			So(lifted[1].Price, ShouldAlmostEqual, initialPrice, initialPrice*idleAmplitudeFraction*2)
			So(compressedQuote.Ask-compressedQuote.Bid, ShouldBeLessThan, initialQuote.Ask-initialQuote.Bid)
		})
	})

	Convey("Given a dump isolated to one symbol", t, func() {
		signal := New([]string{"SIM1/USD", "SIM2/USD"})
		So(signal.Transition(FastDump, "SIM1/USD"), ShouldBeNil)
		selectedSides := []string{}
		controlSides := []string{}

		for samples := range signal.Generate() {
			selectedSides = append(selectedSides, samples[0].Side)
			controlSides = append(controlSides, samples[1].Side)
		}

		Convey("Only its active leg should carry one-sided sell flow", func() {
			for _, side := range selectedSides[:fastLegObservations] {
				So(side, ShouldEqual, "sell")
			}

			So(selectedSides[fastLegObservations:], ShouldResemble,
				[]string{"buy", "sell", "buy", "sell"})
			So(controlSides, ShouldResemble, []string{
				"sell", "buy", "sell", "buy", "sell", "buy",
				"sell", "buy", "sell", "buy", "sell", "buy",
			})
		})
	})

	Convey("Given semantic liquidity conditions on one selected symbol", t, func() {
		thin := New([]string{"SIM1/USD", "SIM2/USD"})
		loaded := New([]string{"SIM1/USD", "SIM2/USD"})
		retreat := New([]string{"SIM1/USD", "SIM2/USD"})
		initial, exists := thin.Quote("SIM1/USD")
		So(exists, ShouldBeTrue)
		So(thin.Transition(ThinLiquidity, "SIM1/USD"), ShouldBeNil)
		So(loaded.Transition(LoadedLiquidity, "SIM1/USD"), ShouldBeNil)
		So(retreat.Transition(LiquidityRetreat, "SIM1/USD"), ShouldBeNil)
		thinQuote, _ := thin.Quote("SIM1/USD")
		loadedQuote, _ := loaded.Quote("SIM1/USD")
		retreatQuote, _ := retreat.Quote("SIM1/USD")

		Convey("They should thin, load, and retreat the real touch", func() {
			So(thinQuote.AskQty, ShouldBeLessThan, initial.AskQty)
			So(loadedQuote.BidQty, ShouldBeGreaterThan, initial.BidQty)
			So(retreatQuote.Bid, ShouldBeLessThan, initial.Bid)
			So(retreatQuote.BidQty, ShouldEqual, initial.BidQty)

			for _, generated := range []*Signal{thin, loaded, retreat} {
				control, _ := generated.Quote("SIM2/USD")
				So(control, ShouldResemble, initial)
			}
		})
	})

	Convey("Given matched directional scenario twins", t, func() {
		type outcome struct {
			first Sample
			last  Sample
		}
		outcomes := map[State]outcome{}

		for _, state := range []State{
			FastPump,
			SlowCadenceLift,
			SmallDisplacementLift,
		} {
			signal := New([]string{"SIM1/USD"})
			So(signal.Transition(state), ShouldBeNil)
			var samples []Sample

			for generated := range signal.Generate() {
				samples = append(samples, generated[0])
			}

			outcomes[state] = outcome{first: samples[0], last: samples[len(samples)-1]}
		}

		Convey("Cadence changes time without changing displacement or volume", func() {
			fast := outcomes[FastPump]
			slow := outcomes[SlowCadenceLift]
			So(fast.last.Price, ShouldEqual, slow.last.Price)
			So(fast.last.Statistics.Volume, ShouldEqual, slow.last.Statistics.Volume)
			So(fast.last.At.Sub(fast.first.At), ShouldBeLessThan,
				slow.last.At.Sub(slow.first.At))
		})

		Convey("Displacement changes price without changing cadence or volume", func() {
			fast := outcomes[FastPump]
			small := outcomes[SmallDisplacementLift]
			So(small.last.Price, ShouldBeLessThan, fast.last.Price)
			So(small.last.Statistics.Volume, ShouldEqual, fast.last.Statistics.Volume)
			So(small.last.At.Sub(small.first.At), ShouldEqual,
				fast.last.At.Sub(fast.first.At))
		})
	})

	Convey("Given a leader followed by successively delayed symbols", t, func() {
		signal := New([]string{"SIM1/USD", "SIM2/USD", "SIM3/USD"})
		So(signal.Transition(LeaderFollower), ShouldBeNil)
		var first []Sample
		var last []Sample

		for generated := range signal.Generate() {
			if first == nil {
				first = generated
			}

			last = generated
		}

		Convey("The anchor should move first and retain the largest lead", func() {
			So(first[0].Price, ShouldBeGreaterThan, first[1].Price)
			So(first[1].Price, ShouldAlmostEqual, initialPrice, PriceIncrement*10)
			So(first[2].Price, ShouldAlmostEqual, initialPrice, PriceIncrement*10)
			So(last[0].Price, ShouldBeGreaterThan, last[1].Price)
			So(last[1].Price, ShouldBeGreaterThan, last[2].Price)
			So(last[2].Price, ShouldBeGreaterThan, initialPrice)
		})
	})

	Convey("Given an adverse subject moving against an advancing cohort", t, func() {
		signal := New([]string{"SIM1/USD", "SIM2/USD", "SIM3/USD"})
		initial := make(map[string]float64)

		for _, symbol := range signal.symbols {
			quote, exists := signal.Quote(symbol)
			So(exists, ShouldBeTrue)
			initial[symbol] = (quote.Bid + quote.Ask) / 2
		}

		So(signal.Transition(AdverseDivergence), ShouldBeNil)
		var last []Sample

		for generated := range signal.Generate() {
			last = generated
		}

		Convey("The subject should decline while both peers advance", func() {
			So(last[0].Price, ShouldBeLessThan, initial[last[0].Symbol])
			So(last[1].Price, ShouldBeGreaterThan, initial[last[1].Symbol])
			So(last[2].Price, ShouldBeGreaterThan, initial[last[2].Symbol])
		})
	})
}

/*
BenchmarkSignal_Transition measures the fixture price and clock generator.
*/
func BenchmarkSignal_Transition(b *testing.B) {
	signal := New([]string{"SIM1/USD", "SIM2/USD", "SIM3/USD"})
	b.ReportAllocs()

	for b.Loop() {
		if err := signal.Transition(Baseline); err != nil {
			b.Fatal(err)
		}

		for samples := range signal.Generate() {
			if samples[0].At.IsZero() {
				b.Fatal("transition timestamp is zero")
			}
		}
	}
}
