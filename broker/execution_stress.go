package broker

import (
	"fmt"
	"math"
	"os"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
ExecutionStressMultiplier scales slippage when measurements show hostile flow or the
structural regime is adverse. Replay passes the tick snapshot; paper uses SymbolStress
plus the latest published market regime.
*/
func ExecutionStressMultiplier(snapshots []types.Measurement) float64 {
	if len(snapshots) == 0 {
		return 1
	}

	stressSNR := 0.0
	stressReadings := 0

	for _, measurement := range snapshots {
		if !executionStressCategory(measurement.Category) {
			continue
		}

		stressReadings++

		if measurement.SNR > stressSNR {
			stressSNR = measurement.SNR
		}
	}

	categoryFactor := 1.0

	if stressReadings > 0 && stressSNR > 0 {
		coverage := float64(stressReadings) / float64(len(snapshots))
		strength := stressSNR / (stressSNR + 1)
		categoryFactor = 1 + coverage*strength
	}

	return categoryFactor * RegimeHostility(perspectives.ClassifyRegime(snapshots).Regime)
}

/*
ExecutionStressFromSymbol approximates replay stress on the paper path from the desk
stress cache and the latest published market regime.
*/
func ExecutionStressFromSymbol(stress SymbolStress) float64 {
	hostile := stress.hostileStress()
	categoryFactor := 1.0

	if hostile > 0 {
		categoryFactor = 1 + hostile/(hostile+1)
	}

	return categoryFactor * RegimeHostility(perspectives.CurrentRegime())
}

/*
RegimeHostility is the structural-regime adverse-selection multiplier.
*/
func RegimeHostility(regime types.Regime) float64 {
	switch regime {
	case types.RegimeBearish:
		return 1.5
	case types.RegimeChoppy:
		return 1.25
	default:
		return 1
	}
}

func executionStressCategory(category types.CategoryType) bool {
	switch category {
	case types.CategoryTurbulent,
		types.CategoryFrenzy,
		types.CategoryLiquidityVacuum,
		types.CategoryLiquidityShock,
		types.CategoryToxicBluff,
		types.CategorySpoofTrap,
		types.CategoryBookThinning,
		types.CategorySystemicHerd,
		types.CategoryMechanicalCollapse,
		types.CategoryDivergentStress,
		types.CategorySystemicSlump:
		return true
	default:
		return false
	}
}

/*
StressedSlippageReplayFill applies replay stress from a measurement window snapshot.
Optimizer replay calls this instead of maintaining a parallel fill implementation.
*/
func StressedSlippageReplayFill(
	quote Quote,
	side trading.Side,
	qty float64,
	snapshots []types.Measurement,
) (FillQuote, error) {
	fill, err := SlippageFill(quote, side, qty)

	if err != nil {
		return FillQuote{}, err
	}

	if !viper.GetBool("trading.replay.execution_stress_enabled") {
		return fill, nil
	}

	return applyExecutionStress(fill, quote, side, ExecutionStressMultiplier(snapshots)), nil
}

func applyExecutionStress(
	fill FillQuote,
	quote Quote,
	side trading.Side,
	multiplier float64,
) FillQuote {
	slippagePct := fill.SlippageBps / 10_000
	slippagePct *= multiplier

	if hasBookDepth(quote) && fill.DepthCoverage < 1 {
		shortfall := 1 - fill.DepthCoverage
		base := slippagePct

		if base <= 0 {
			base = defaultPaperSlippagePct()
		}

		slippagePct += shortfall * base * multiplier * depthShortfallStressMultiplier()
	}

	reference := quote.Last

	if reference <= 0 && quote.Bid > 0 && quote.Ask > 0 {
		reference = (quote.Bid + quote.Ask) / 2
	}

	price := fillPriceFromSlippagePct(side, reference, slippagePct)

	return FillQuote{
		Price:         price,
		SlippageBps:   slippagePct * 10_000,
		DepthCoverage: fill.DepthCoverage,
	}
}

/*
StressedSlippageFill walks the book and inflates slippage for hostile flow, regime,
and depth shortfall — matching the replay ledger's taker model.
*/
func StressedSlippageFill(
	quote Quote,
	side trading.Side,
	qty float64,
	stress SymbolStress,
) (FillQuote, error) {
	fill, err := SlippageFill(quote, side, qty)

	if err != nil {
		return FillQuote{}, err
	}

	if !viper.GetBool("trading.replay.execution_stress_enabled") {
		return fill, nil
	}

	return applyExecutionStress(fill, quote, side, ExecutionStressFromSymbol(stress)), nil
}

func defaultPaperSlippagePct() float64 {
	slippageBps := viper.GetFloat64("trading.paper.slippage_bps")

	if slippageBps > 0 {
		return slippageBps / 10_000
	}

	return 0
}

func depthShortfallStressMultiplier() float64 {
	multiplier := viper.GetFloat64("trading.replay.depth_shortfall_stress")

	if multiplier > 0 {
		return multiplier
	}

	return 8
}

func fillPriceFromSlippagePct(side trading.Side, reference, slippagePct float64) float64 {
	if side == trading.Sell {
		return reference * (1 - slippagePct)
	}

	return reference * (1 + slippagePct)
}

func hasBookDepth(quote Quote) bool {
	return len(quote.Book.Bids) > 0 || len(quote.Book.Asks) > 0
}

/*
EffectiveNetworkLatency reads runs/network_latency.json and returns the p95 sample.
*/
func EffectiveNetworkLatency() time.Duration {
	latencyFile, err := os.Open("runs/network_latency.json")

	if err != nil {
		errnie.Error(err)
		return 0
	}
	defer latencyFile.Close()

	values := make([]int64, 0, 64)

	for {
		var value int64
		_, scanErr := fmt.Fscanf(latencyFile, "%d\n", &value)

		if scanErr != nil {
			break
		}

		if value > 0 {
			values = append(values, value)
		}
	}

	return time.Duration(percentileInt64(values, 0.95))
}

func percentileInt64(values []int64, quantile float64) int64 {
	if len(values) == 0 {
		return 0
	}

	sorted := append([]int64(nil), values...)
	sortInt64(sorted)

	index := int(math.Floor(float64(len(sorted)-1) * quantile))

	if index < 0 {
		index = 0
	}

	return sorted[index]
}

func sortInt64(values []int64) {
	for left := 1; left < len(values); left++ {
		cursor := values[left]
		walk := left

		for walk > 0 && values[walk-1] > cursor {
			values[walk] = values[walk-1]
			walk--
		}

		values[walk] = cursor
	}
}
