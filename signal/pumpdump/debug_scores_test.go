package pumpdump

import (
	"context"
	"fmt"
	"testing"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
)

func TestDebugVerticalScores(t *testing.T) {
	signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))

	defer func() {
		_ = signal.Close()
	}()

	volume, vwap, last, bid, ask, changePct := verticalIgnitionTicker()
	warmupTickerFrames(signal, "ETH/EUR", 59, 100, vwap, 10000, 9990, 10010, 0)
	result := measureTickerFrame(signal, "ETH/EUR", volume, vwap, last, bid, ask, changePct)

	keys := []string{
		"ignition", "compression", "trend", "exhaustion",
		"rvol", "precursor", "value", "category",
	}

	for _, key := range keys {
		fmt.Printf("%s=%v\n", key, datura.Peek[float64](result, "output", key))
	}
}

func TestDebugScenarioScores(t *testing.T) {
	scenarios := []struct {
		name                                        string
		symbol                                      string
		warmupTicks                                 int
		volumeStep, vwap, last, bid, ask, changePct float64
		warmupLast, warmupBid, warmupAsk            float64
	}{
		{
			name: "coiled", symbol: "BTC/EUR", warmupTicks: 59, volumeStep: 120,
			vwap: 10000, last: 10050, bid: 10050.0001, ask: 10050.0002, changePct: 0.05,
			warmupLast: 10050, warmupBid: 10040, warmupAsk: 10060,
		},
		{
			name: "organic", symbol: "TREND/EUR", warmupTicks: 59, volumeStep: 100,
			vwap: 10000, last: 12500, bid: 12490, ask: 12510, changePct: 0.4,
			warmupLast: 12400, warmupBid: 12490, warmupAsk: 12510,
		},
		{
			name: "faded", symbol: "FADE/EUR", warmupTicks: 59, volumeStep: 200,
			vwap: 10000, last: 10100, bid: 10080, ask: 10120, changePct: 0.05,
			warmupLast: 10100, warmupBid: 10095, warmupAsk: 10105,
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))

			defer func() {
				_ = signal.Close()
			}()

			warmupTickerFrames(
				signal, scenario.symbol, scenario.warmupTicks, scenario.volumeStep,
				scenario.vwap, scenario.warmupLast, scenario.warmupBid, scenario.warmupAsk, 0,
			)

			var volume float64

			switch scenario.name {
			case "coiled":
				volume, _, _, _, _, _ = coiledCompressionTicker()
			case "organic":
				volume, _, _, _, _, _ = organicTrendTicker()
			case "faded":
				volume, _, _, _, _, _ = fadedExhaustionTicker()
			}

			result := measureTickerFrame(
				signal, scenario.symbol, volume, scenario.vwap,
				scenario.last, scenario.bid, scenario.ask, scenario.changePct,
			)

			fmt.Printf("%s category=%v compression=%v metric=%v trend=%v spread=%v\n",
				scenario.name,
				datura.Peek[float64](result, "output", "category"),
				datura.Peek[float64](result, "output", "compression"),
				datura.Peek[float64](result, "output", "compression"),
				datura.Peek[float64](result, "output", "trend"),
				datura.Peek[float64](result, "output", "spread"),
			)
		})
	}
}
