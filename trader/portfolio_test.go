package trader

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/logic"
)

func testPortfolio() *Portfolio {
	viper.Set("trading.slots.normal", 4)
	viper.Set("trading.entry.opportunity_slot_count", 2)
	viper.Set("trading.stop.trailing_offset_bps", 100)
	viper.Set("trading.stop.min_offset_bps", 20)
	viper.Set("trading.stop.max_offset_bps", 500)
	viper.Set("trading.stop.momentum_decay_fraction", 0.6)
	viper.Set("trading.stop.stagnation_max_touches", 3)
	viper.Set("trading.stop.stagnation_zone_fraction", 0.1)
	viper.Set("trading.stop.take_profit_arm_pct", 0.06)
	viper.Set("trading.stop.take_profit_tight_offset_bps", 150)
	viper.Set("trading.stop.take_profit_cap_pct", 0.20)
	viper.Set("trading.stop.breakout_threshold_pct", 0.10)
	viper.Set("trading.stop.breakout_hold_probability", 0.55)
	viper.Set("trading.paper.taker_fee_bps", 40)
	viper.Set("trading.paper.slippage_bps", 2)

	portfolio, err := NewPortfolio(nil)

	if err != nil {
		panic(err)
	}

	return portfolio
}

func buy(symbol string) *logic.Action {
	return &logic.Action{
		Symbol:   symbol,
		Side:     "buy",
		Score:    0.5,
		Fraction: 0.05,
		Price:    *decimal.NewFromFloat64(100),
	}
}

func sell(symbol string) *logic.Action {
	return &logic.Action{Symbol: symbol, Side: "sell", Score: 0.5, Price: *decimal.NewFromFloat64(100)}
}

// held builds a holdings snapshot. FeeRate mirrors the 40bps taker configured in
// testPortfolio so round-trip friction (2*fee + 2*slippage) matches the values
// the exit-timing assertions were written against.
func held(symbol string, returnPct float64) map[string]broker.PositionData {
	return map[string]broker.PositionData{
		symbol: {
			Symbol:    symbol,
			ReturnPct: returnPct,
			FeeRate:   0.004,
			Spread:    *decimal.NewFromFloat64(0.0004),
		},
	}
}

func mom(symbol string, score float64) map[string]float64 {
	return map[string]float64{symbol: score}
}

func cont(symbol string, probability float64) map[string]float64 {
	return map[string]float64{symbol: probability}
}

func intentFor(intents []tradeIntent, symbol string) (tradeIntent, bool) {
	for _, intent := range intents {
		if intent.symbol == symbol {
			return intent, true
		}
	}

	return tradeIntent{}, false
}

func TestPortfolioTakeProfit(testingTB *testing.T) {
	Convey("Given a large winner", testingTB, func() {
		Convey("When the return reaches the absolute cap", func() {
			portfolio := testPortfolio()
			portfolio.Reconcile([]*logic.Action{buy("ZEUS/USD")}, nil, nil, nil)
			// Peak climbs straight to +25%, past the 20% cap.
			intents := portfolio.Reconcile(nil, held("ZEUS/USD", 0.25), nil, nil)

			Convey("Then it is banked outright at the cap", func() {
				exit, ok := intentFor(intents, "ZEUS/USD")
				So(ok, ShouldBeTrue)
				So(exit.kind, ShouldEqual, intentExit)
				So(exit.reason, ShouldEqual, "take_profit_cap")
			})
		})

		Convey("When a peak above the arm threshold gives back past the tight offset", func() {
			portfolio := testPortfolio()
			portfolio.Reconcile([]*logic.Action{buy("ZEUS/USD")}, nil, nil, nil)
			// Peak at +8% arms the tightened trail (tight offset 1.5%).
			portfolio.Reconcile(nil, held("ZEUS/USD", 0.08), nil, nil)

			Convey("Then a giveback beyond the tight offset banks it near the high", func() {
				// +6.4% is 1.6% below the 8% peak — beyond the 1.5% tight offset.
				intents := portfolio.Reconcile(nil, held("ZEUS/USD", 0.064), nil, nil)
				exit, ok := intentFor(intents, "ZEUS/USD")
				So(ok, ShouldBeTrue)
				So(exit.reason, ShouldEqual, "take_profit_trail")
			})

			Convey("Then a giveback within the tight offset keeps running", func() {
				// +6.7% is 1.3% below peak — within the 1.5% tight offset.
				intents := portfolio.Reconcile(nil, held("ZEUS/USD", 0.067), nil, nil)
				_, ok := intentFor(intents, "ZEUS/USD")
				So(ok, ShouldBeFalse)
			})
		})

		Convey("When the peak is below the arm threshold", func() {
			portfolio := testPortfolio()
			portfolio.Reconcile([]*logic.Action{buy("ZEUS/USD")}, nil, nil, nil)
			// Peak at +5% — below the 6% arm, so the loose 1% trail still governs.
			portfolio.Reconcile(nil, held("ZEUS/USD", 0.05), nil, nil)

			Convey("Then a 1.6% giveback exits on the loose trail as normal", func() {
				// Not armed, so the standard 1% trailing offset governs: +3.4% is
				// 1.6% below the +5% peak, tripping the ordinary trailing stop.
				intents := portfolio.Reconcile(nil, held("ZEUS/USD", 0.034), nil, nil)
				exit, ok := intentFor(intents, "ZEUS/USD")
				So(ok, ShouldBeTrue)
				So(exit.reason, ShouldEqual, "trailing_stop")
			})
		})
	})
}

func TestPortfolioBreakout(testingTB *testing.T) {
	Convey("Given a position broken out past the breakout threshold", testingTB, func() {
		// Breakout engages above +10%; hold probability bar is 0.55.
		setup := func() *Portfolio {
			portfolio := testPortfolio()
			portfolio.Reconcile([]*logic.Action{buy("ZEUS/USD")}, nil, nil, nil)
			return portfolio
		}

		Convey("When continuation predicts the move keeps running", func() {
			portfolio := setup()
			intents := portfolio.Reconcile(nil, held("ZEUS/USD", 0.12), nil, cont("ZEUS/USD", 0.8))

			Convey("Then it holds and keeps riding the breakout", func() {
				_, ok := intentFor(intents, "ZEUS/USD")
				So(ok, ShouldBeFalse)
			})
		})

		Convey("When the directional edge has faded (below the hold bar)", func() {
			portfolio := setup()
			intents := portfolio.Reconcile(nil, held("ZEUS/USD", 0.12), nil, cont("ZEUS/USD", 0.5))

			Convey("Then it takes the breakout gain", func() {
				exit, ok := intentFor(intents, "ZEUS/USD")
				So(ok, ShouldBeTrue)
				So(exit.reason, ShouldEqual, "breakout_take")
			})
		})

		Convey("When it was held on an up-prediction that then reverses down", func() {
			portfolio := setup()
			// First tick: strong up read → hold, mark breakoutHeld.
			held1 := portfolio.Reconcile(nil, held("ZEUS/USD", 0.12), nil, cont("ZEUS/USD", 0.8))
			_, ok1 := intentFor(held1, "ZEUS/USD")
			So(ok1, ShouldBeFalse)

			// Next tick: prediction flips to down while still in profit.
			intents := portfolio.Reconcile(nil, held("ZEUS/USD", 0.11), nil, cont("ZEUS/USD", 0.3))

			Convey("Then it banks the lesser profit on the reversal", func() {
				exit, ok := intentFor(intents, "ZEUS/USD")
				So(ok, ShouldBeTrue)
				So(exit.reason, ShouldEqual, "breakout_reversal")
			})
		})

		Convey("When it is below the breakout threshold", func() {
			portfolio := setup()
			// +5% is below +10%: breakout logic does not engage, even on a weak read.
			intents := portfolio.Reconcile(nil, held("ZEUS/USD", 0.05), nil, cont("ZEUS/USD", 0.2))

			Convey("Then breakout does not fire", func() {
				exit, ok := intentFor(intents, "ZEUS/USD")
				if ok {
					So(exit.reason, ShouldNotEqual, "breakout_take")
					So(exit.reason, ShouldNotEqual, "breakout_reversal")
				}
			})
		})
	})
}

func TestPortfolioTrailingStopExit(testingTB *testing.T) {
	Convey("Given a filled position that ran up then bled back", testingTB, func() {
		portfolio := testPortfolio()

		enter, _ := intentFor(portfolio.Reconcile([]*logic.Action{buy("TAO/USD")}, nil, nil, nil), "TAO/USD")
		So(enter.kind, ShouldEqual, intentEnter)

		// Fill confirmed at +2%; peak return is now 0.02.
		portfolio.Reconcile(nil, held("TAO/USD", 0.02), nil, nil)

		Convey("When it drops more than the trailing offset below its peak", func() {
			intents := portfolio.Reconcile(nil, held("TAO/USD", 0.005), nil, nil)

			Convey("Then a trailing-stop exit fires", func() {
				exit, ok := intentFor(intents, "TAO/USD")
				So(ok, ShouldBeTrue)
				So(exit.kind, ShouldEqual, intentExit)
				So(exit.reason, ShouldEqual, "trailing_stop")
			})
		})

		Convey("When it holds within the trailing offset", func() {
			intents := portfolio.Reconcile(nil, held("TAO/USD", 0.015), nil, nil)

			Convey("Then it keeps holding", func() {
				_, ok := intentFor(intents, "TAO/USD")
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func TestPortfolioReversalExit(testingTB *testing.T) {
	Convey("Given a filled position in profit past round-trip friction", testingTB, func() {
		portfolio := testPortfolio()

		portfolio.Reconcile([]*logic.Action{buy("TAO/USD")}, nil, nil, nil)
		portfolio.Reconcile(nil, held("TAO/USD", 0.02), nil, nil)

		Convey("When the read reverses to down-conviction", func() {
			intents := portfolio.Reconcile([]*logic.Action{sell("TAO/USD")}, held("TAO/USD", 0.02), nil, nil)

			Convey("Then a thesis-reversal exit fires", func() {
				exit, ok := intentFor(intents, "TAO/USD")
				So(ok, ShouldBeTrue)
				So(exit.kind, ShouldEqual, intentExit)
				So(exit.reason, ShouldEqual, "thesis_reversal")
			})
		})

		Convey("When the reversal comes before friction is cleared", func() {
			portfolioEarly := testPortfolio()
			portfolioEarly.Reconcile([]*logic.Action{buy("SUI/USD")}, nil, nil, nil)
			portfolioEarly.Reconcile(nil, held("SUI/USD", 0.001), nil, nil)
			intents := portfolioEarly.Reconcile([]*logic.Action{sell("SUI/USD")}, held("SUI/USD", 0.001), nil, nil)

			Convey("Then it holds instead of paying fees to churn", func() {
				_, ok := intentFor(intents, "SUI/USD")
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func TestPortfolioMomentumDecayExit(testingTB *testing.T) {
	Convey("Given a filled position in profit whose field momentum peaked", testingTB, func() {
		portfolio := testPortfolio()

		portfolio.Reconcile([]*logic.Action{buy("BLUR/USD")}, nil, nil, nil)
		// Fill confirmed at +2% with strong momentum; peakMomentum is now 1.0.
		portfolio.Reconcile(nil, held("BLUR/USD", 0.02), mom("BLUR/USD", 1.0), nil)

		Convey("When momentum decays below the configured fraction of its peak", func() {
			// Still profitable past friction, but the move's energy has died to
			// 0.5 (below 0.6 * peak). Price has not given back enough to trip the
			// price trailing stop.
			intents := portfolio.Reconcile(nil, held("BLUR/USD", 0.019), mom("BLUR/USD", 0.5), nil)

			Convey("Then a momentum-decay exit fires", func() {
				exit, ok := intentFor(intents, "BLUR/USD")
				So(ok, ShouldBeTrue)
				So(exit.kind, ShouldEqual, intentExit)
				So(exit.reason, ShouldEqual, "momentum_decay")
			})
		})

		Convey("When momentum stays above the decay threshold", func() {
			intents := portfolio.Reconcile(nil, held("BLUR/USD", 0.019), mom("BLUR/USD", 0.9), nil)

			Convey("Then it keeps following the move up", func() {
				_, ok := intentFor(intents, "BLUR/USD")
				So(ok, ShouldBeFalse)
			})
		})

		Convey("When momentum makes a new high", func() {
			// A higher reading re-ratchets the peak, so a later 0.7 is above
			// 0.6 * the new peak (1.5) and must not exit.
			So(portfolio.Reconcile(nil, held("BLUR/USD", 0.03), mom("BLUR/USD", 1.5), nil), ShouldBeEmpty)
			intents := portfolio.Reconcile(nil, held("BLUR/USD", 0.029), mom("BLUR/USD", 1.0), nil)

			Convey("Then the raised peak keeps it holding", func() {
				_, ok := intentFor(intents, "BLUR/USD")
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func TestPortfolioMomentumDecayDoesNotExitBeforeFriction(testingTB *testing.T) {
	Convey("Given a filled position that has not cleared round-trip friction", testingTB, func() {
		portfolio := testPortfolio()

		portfolio.Reconcile([]*logic.Action{buy("SUI/USD")}, nil, nil, nil)
		portfolio.Reconcile(nil, held("SUI/USD", 0.001), mom("SUI/USD", 1.0), nil)

		Convey("When momentum decays while still underwater on costs", func() {
			intents := portfolio.Reconcile(nil, held("SUI/USD", 0.001), mom("SUI/USD", 0.2), nil)

			Convey("Then no momentum-decay exit fires", func() {
				_, ok := intentFor(intents, "SUI/USD")
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func TestPortfolioExitEvalInstrumentation(testingTB *testing.T) {
	Convey("Given a held position and audit exit tracing enabled", testingTB, func() {
		viper.Set("system.audit.decisions", true)
		defer viper.Set("system.audit.decisions", false)

		file := filepath.Join(testingTB.TempDir(), "audit.jsonl")
		recorder, err := audit.NewRecorder(file)
		So(err, ShouldBeNil)

		viper.Set("trading.slots.normal", 4)
		viper.Set("trading.entry.opportunity_slot_count", 2)
		viper.Set("trading.stop.trailing_offset_bps", 100)
		viper.Set("trading.stop.min_offset_bps", 20)
		viper.Set("trading.stop.max_offset_bps", 500)
		viper.Set("trading.stop.momentum_decay_fraction", 0.6)
		viper.Set("trading.paper.taker_fee_bps", 40)
		viper.Set("trading.paper.slippage_bps", 2)

		portfolio, err := NewPortfolio(recorder)
		So(err, ShouldBeNil)

		portfolio.Reconcile([]*logic.Action{buy("BLUR/USD")}, nil, nil, nil)
		portfolio.Reconcile(nil, held("BLUR/USD", 0.02), mom("BLUR/USD", 1.0), nil)

		Convey("When momentum is still high (no exit)", func() {
			portfolio.Reconcile(nil, held("BLUR/USD", 0.019), mom("BLUR/USD", 0.9), nil)
			So(recorder.Close(), ShouldBeNil)

			Convey("Then an exit_eval event records the blocking gate", func() {
				data, readErr := os.ReadFile(file)
				So(readErr, ShouldBeNil)
				So(string(data), ShouldContainSubstring, "\"exit_eval\"")
				So(string(data), ShouldContainSubstring, "momentum_still_high")
				So(string(data), ShouldContainSubstring, "\"peakMomentum\":1")
			})
		})

		Convey("When the momentum signal is absent", func() {
			portfolio.Reconcile(nil, held("BLUR/USD", 0.019), nil, nil)
			So(recorder.Close(), ShouldBeNil)

			Convey("Then the eval records no_momentum_signal", func() {
				data, readErr := os.ReadFile(file)
				So(readErr, ShouldBeNil)
				So(string(data), ShouldContainSubstring, "no_momentum_signal")
			})
		})
	})
}

func TestPortfolioStops(testingTB *testing.T) {
	Convey("Given a held position whose peak return has ratcheted", testingTB, func() {
		portfolio := testPortfolio()

		// Enter at price 100, confirm the fill, then let the peak reach +2%.
		portfolio.Reconcile([]*logic.Action{buy("TAO/USD")}, nil, nil, nil)
		portfolio.Reconcile(nil, held("TAO/USD", 0.02), mom("TAO/USD", 1.0), nil)

		Convey("When the trailing stop overlay is read", func() {
			stops := portfolio.Stops()

			Convey("Then the stop price is entry scaled by peakReturn minus offset", func() {
				stop, ok := stops["TAO/USD"]
				So(ok, ShouldBeTrue)
				// offset = trailing_offset_bps 100 = 0.01 (within clamp bounds).
				// stopReturn = 0.02 - 0.01 = 0.01; stop = 100 * 1.01 = 101.
				So(stop.PeakReturn, ShouldAlmostEqual, 0.02)
				So(stop.StopReturn, ShouldAlmostEqual, 0.01)
				So(stop.StopPrice, ShouldAlmostEqual, 101.0)
			})

			Convey("Then the momentum overlay reflects proximity to the decay floor", func() {
				stop := stops["TAO/USD"]
				// peakMomentum 1.0, momentumDecay 0.6 → floor 0.6.
				So(stop.MomentumActive, ShouldBeTrue)
				So(stop.PeakMomentum, ShouldAlmostEqual, 1.0)
				So(stop.MomentumFloor, ShouldAlmostEqual, 0.6)
				// At the peak, health is 1.0.
				So(stop.MomentumHealth, ShouldAlmostEqual, 1.0)
			})
		})

		Convey("When momentum has decayed toward the floor", func() {
			// peakMomentum stays 1.0; latest reading 0.8 → health = (0.8-0.6)/(1-0.6) = 0.5.
			portfolio.Reconcile(nil, held("TAO/USD", 0.019), mom("TAO/USD", 0.8), nil)
			stops := portfolio.Stops()

			Convey("Then momentum health drops proportionally", func() {
				So(stops["TAO/USD"].MomentumHealth, ShouldAlmostEqual, 0.5)
			})
		})
	})

	Convey("Given a pending (unfilled) entry", testingTB, func() {
		portfolio := testPortfolio()
		portfolio.Reconcile([]*logic.Action{buy("SUI/USD")}, nil, nil, nil)

		Convey("When stops are read before the fill lands", func() {
			stops := portfolio.Stops()

			Convey("Then the pending position has no stop overlay yet", func() {
				_, ok := stops["SUI/USD"]
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func TestPortfolioStagnationExit(testingTB *testing.T) {
	Convey("Given a filled position that peaked at +1.7% and keeps revisiting that zone", testingTB, func() {
		portfolio := testPortfolio()

		// Enter at price 100, fill confirmed at +1.7%.
		portfolio.Reconcile([]*logic.Action{buy("TAO/USD")}, nil, nil, nil)
		portfolio.Reconcile(nil, held("TAO/USD", 0.017), nil, nil)

		Convey("When it touches the peak zone once", func() {
			// First touch: return at 1.6% (within 10% zone of 1.7% peak).
			intents := portfolio.Reconcile(nil, held("TAO/USD", 0.016), nil, nil)
			_, ok := intentFor(intents, "TAO/USD")

			Convey("Then it holds (needs 3 touches)", func() {
				So(ok, ShouldBeFalse)
			})
		})

		Convey("When it touches the peak zone 3 times", func() {
			portfolio.Reconcile(nil, held("TAO/USD", 0.016), nil, nil)
			portfolio.Reconcile(nil, held("TAO/USD", 0.0165), nil, nil)
			intents := portfolio.Reconcile(nil, held("TAO/USD", 0.016), nil, nil)

			Convey("Then a stagnation exit fires", func() {
				exit, ok := intentFor(intents, "TAO/USD")
				So(ok, ShouldBeTrue)
				So(exit.kind, ShouldEqual, intentExit)
				So(exit.reason, ShouldEqual, "stagnation")
			})
		})

		Convey("When it makes a new high after touches", func() {
			portfolio.Reconcile(nil, held("TAO/USD", 0.016), nil, nil)
			portfolio.Reconcile(nil, held("TAO/USD", 0.0165), nil, nil)
			// New peak at +2.0% resets the touch counter.
			portfolio.Reconcile(nil, held("TAO/USD", 0.02), nil, nil)
			// Now it needs 3 more touches above 1.8% (2.0 - 0.2).
			portfolio.Reconcile(nil, held("TAO/USD", 0.019), nil, nil)
			portfolio.Reconcile(nil, held("TAO/USD", 0.019), nil, nil)
			intents := portfolio.Reconcile(nil, held("TAO/USD", 0.019), nil, nil)

			Convey("Then it holds (new peak reset the counter)", func() {
				_, ok := intentFor(intents, "TAO/USD")
				So(ok, ShouldBeFalse)
			})
		})

		Convey("When it drops below the zone between touches", func() {
			portfolio.Reconcile(nil, held("TAO/USD", 0.016), nil, nil)
			// Drops to 1.0% (below zone), then comes back.
			portfolio.Reconcile(nil, held("TAO/USD", 0.01), nil, nil)
			portfolio.Reconcile(nil, held("TAO/USD", 0.016), nil, nil)
			portfolio.Reconcile(nil, held("TAO/USD", 0.016), nil, nil)
			intents := portfolio.Reconcile(nil, held("TAO/USD", 0.016), nil, nil)

			Convey("Then it still needs 3 touches (drop resets pastPeakTouch)", func() {
				_, ok := intentFor(intents, "TAO/USD")
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func TestPortfolioStagnationDoesNotExitBeforeFriction(testingTB *testing.T) {
	Convey("Given a filled position below round-trip friction", testingTB, func() {
		portfolio := testPortfolio()

		portfolio.Reconcile([]*logic.Action{buy("SUI/USD")}, nil, nil, nil)
		portfolio.Reconcile(nil, held("SUI/USD", 0.005), nil, nil)

		Convey("When it revisits its peak zone", func() {
			portfolio.Reconcile(nil, held("SUI/USD", 0.0045), nil, nil)
			portfolio.Reconcile(nil, held("SUI/USD", 0.0045), nil, nil)
			intents := portfolio.Reconcile(nil, held("SUI/USD", 0.0045), nil, nil)

			Convey("Then no stagnation exit fires", func() {
				_, ok := intentFor(intents, "SUI/USD")
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func TestPortfolioNoReentry(testingTB *testing.T) {
	Convey("Given a symbol already held", testingTB, func() {
		portfolio := testPortfolio()
		portfolio.Reconcile([]*logic.Action{buy("XRP/USD")}, nil, nil, nil)

		Convey("When another buy read arrives for the same symbol", func() {
			intents := portfolio.Reconcile([]*logic.Action{buy("XRP/USD")}, held("XRP/USD", 0.01), nil, nil)

			Convey("Then it does not stack a second position", func() {
				_, ok := intentFor(intents, "XRP/USD")
				So(ok, ShouldBeFalse)
			})
		})
	})
}

func BenchmarkPortfolioReconcile(benchmarkTB *testing.B) {
	benchmarkTB.ReportAllocs()

	for benchmarkTB.Loop() {
		portfolio := testPortfolio()
		portfolio.Reconcile([]*logic.Action{buy("BLUR/USD")}, nil, nil, nil)
		portfolio.Reconcile(nil, held("BLUR/USD", 0.02), mom("BLUR/USD", 1.0), nil)
		portfolio.Reconcile(nil, held("BLUR/USD", 0.019), mom("BLUR/USD", 0.9), nil)
		portfolio.Reconcile(nil, held("BLUR/USD", 0.02), mom("BLUR/USD", 1.1), nil)
		portfolio.Reconcile(nil, held("BLUR/USD", 0.018), mom("BLUR/USD", 0.5), nil)
	}
}
