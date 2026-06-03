package integration

import (
	"time"

	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
)

/*
SignalFixtureKey names a deterministic synthetic market tape for category validation.
*/
type SignalFixtureKey string

const (
	FixtureCVDAggressiveDrive         SignalFixtureKey = "cvd.aggressive_drive"
	FixtureCVDHiddenAbsorption        SignalFixtureKey = "cvd.hidden_absorption"
	FixtureCVDStochasticBalance       SignalFixtureKey = "cvd.stochastic_balance"
	FixtureCVDVolumeStarvation        SignalFixtureKey = "cvd.volume_starvation"
	FixtureFluidLaminar               SignalFixtureKey = "fluid.laminar"
	FixtureFluidTurbulent             SignalFixtureKey = "fluid.turbulent"
	FixtureFluidInertial              SignalFixtureKey = "fluid.inertial"
	FixtureFluidViscous               SignalFixtureKey = "fluid.viscous"
	FixtureHawkesFrenzy               SignalFixtureKey = "hawkes.frenzy"
	FixtureHawkesSaturation           SignalFixtureKey = "hawkes.saturation"
	FixtureHawkesOrganic              SignalFixtureKey = "hawkes.organic"
	FixtureHawkesExhaustion           SignalFixtureKey = "hawkes.exhaustion"
	FixtureDepthflowLoadedImbalance   SignalFixtureKey = "depthflow.loaded_imbalance"
	FixtureDepthflowSpoofTrap         SignalFixtureKey = "depthflow.spoof_trap"
	FixtureDepthflowBookThinning      SignalFixtureKey = "depthflow.book_thinning"
	FixtureDepthflowDenseNeutrality   SignalFixtureKey = "depthflow.dense_neutrality"
	FixtureSentimentSystemicSlump     SignalFixtureKey = "sentiment.systemic_slump"
	FixtureSentimentRiskOnSurge       SignalFixtureKey = "sentiment.risk_on_surge"
	FixtureSentimentDivergentMove     SignalFixtureKey = "sentiment.divergent_move"
	FixtureLiquidityRobust            SignalFixtureKey = "liquidity.robust"
	FixtureLiquidityMedianDepth       SignalFixtureKey = "liquidity.median_depth"
	FixtureLiquidityExtremeScarcity   SignalFixtureKey = "liquidity.extreme_scarcity"
	FixturePumpdumpVerticalIgnition   SignalFixtureKey = "pumpdump.vertical_ignition"
	FixturePumpdumpCoiledCompression  SignalFixtureKey = "pumpdump.coiled_compression"
	FixturePumpdumpOrganicTrend       SignalFixtureKey = "pumpdump.organic_trend"
	FixturePumpdumpFadedExhaustion    SignalFixtureKey = "pumpdump.faded_exhaustion"
	FixtureExhaustMechanicalCollapse  SignalFixtureKey = "exhaust.mechanical_collapse"
	FixtureExhaustFragileExpansion    SignalFixtureKey = "exhaust.fragile_expansion"
	FixtureExhaustThermalExhaustion   SignalFixtureKey = "exhaust.thermal_exhaustion"
	FixtureExhaustActiveReversal      SignalFixtureKey = "exhaust.active_reversal"
	FixtureCausalEndogenousAlpha      SignalFixtureKey = "causal.endogenous_alpha"
	FixtureCausalSystemicBeta         SignalFixtureKey = "causal.systemic_beta"
	FixtureCausalLiquidityShock       SignalFixtureKey = "causal.liquidity_shock"
	FixtureCausalCausalNoise          SignalFixtureKey = "causal.causal_noise"
	FixtureLeadlagAnchorStall         SignalFixtureKey = "leadlag.anchor_stall"
	FixtureLeadlagInefficientLag      SignalFixtureKey = "leadlag.inefficient_lag"
	FixtureLeadlagSynchronizedDrift   SignalFixtureKey = "leadlag.synchronized_drift"
	FixtureLeadlagDecoupledMove       SignalFixtureKey = "leadlag.decoupled_move"
	FixtureCorrelationSystemicHerd    SignalFixtureKey = "correlation.systemic_herd"
	FixtureCorrelationDecoupledAlpha  SignalFixtureKey = "correlation.decoupled_alpha"
	FixtureCorrelationStochasticNoise SignalFixtureKey = "correlation.stochastic_noise"
	FixtureCorrelationDivergentStress SignalFixtureKey = "correlation.divergent_stress"
	FixtureToxicityToxicBluff         SignalFixtureKey = "toxicity.toxic_bluff"
	FixtureToxicityLiquidityVacuum    SignalFixtureKey = "toxicity.liquidity_vacuum"
	FixtureToxicityHardSupport        SignalFixtureKey = "toxicity.hard_support"
)

/*
ApplySignalFixture replays the synthetic tape associated with a validation probe.
*/
func (builder *CaptureBuilder) ApplySignalFixture(fixture SignalFixtureKey) {
	switch fixture {
	case FixtureCVDAggressiveDrive:
		builder.appendCVDAggressiveDrive()
	case FixtureCVDHiddenAbsorption:
		builder.appendCVDHiddenAbsorption()
	case FixtureCVDStochasticBalance:
		builder.appendCVDStochasticBalance()
	case FixtureCVDVolumeStarvation:
		builder.appendCVDVolumeStarvation()
	case FixtureFluidLaminar:
		builder.appendFluidLaminar()
	case FixtureFluidTurbulent:
		builder.appendFluidTurbulent()
	case FixtureFluidInertial:
		builder.appendFluidInertial()
	case FixtureFluidViscous:
		builder.appendFluidViscous()
	case FixtureHawkesFrenzy:
		builder.appendHawkesFrenzy()
	case FixtureHawkesSaturation:
		builder.appendHawkesSaturation()
	case FixtureHawkesOrganic:
		builder.appendHawkesOrganic()
	case FixtureHawkesExhaustion:
		builder.appendHawkesExhaustion()
	case FixtureDepthflowLoadedImbalance:
		builder.appendDepthflowLoadedImbalance()
	case FixtureDepthflowSpoofTrap:
		builder.appendDepthflowSpoofTrap()
	case FixtureDepthflowBookThinning:
		builder.appendDepthflowBookThinning()
	case FixtureDepthflowDenseNeutrality:
		builder.appendDepthflowDenseNeutrality()
	case FixtureSentimentSystemicSlump:
		builder.appendSentimentSystemicSlump()
	case FixtureSentimentRiskOnSurge:
		builder.appendSentimentRiskOnSurge()
	case FixtureSentimentDivergentMove:
		builder.appendSentimentDivergentMove()
	case FixtureLiquidityRobust:
		builder.appendLiquidityRobust()
	case FixtureLiquidityMedianDepth:
		builder.appendLiquidityMedianDepth()
	case FixtureLiquidityExtremeScarcity:
		builder.appendLiquidityExtremeScarcity()
	case FixturePumpdumpVerticalIgnition:
		builder.appendPumpdumpVerticalIgnition()
	case FixturePumpdumpCoiledCompression:
		builder.appendPumpdumpCoiledCompression()
	case FixturePumpdumpOrganicTrend:
		builder.appendPumpdumpOrganicTrend()
	case FixturePumpdumpFadedExhaustion:
		builder.appendPumpdumpFadedExhaustion()
	case FixtureExhaustMechanicalCollapse:
		builder.appendExhaustMechanicalCollapse()
	case FixtureExhaustFragileExpansion:
		builder.appendExhaustFragileExpansion()
	case FixtureExhaustThermalExhaustion:
		builder.appendExhaustThermalExhaustion()
	case FixtureExhaustActiveReversal:
		builder.appendExhaustActiveReversal()
	case FixtureCausalEndogenousAlpha:
		builder.appendCausalEndogenousAlpha()
	case FixtureCausalSystemicBeta:
		builder.appendCausalSystemicBeta()
	case FixtureCausalLiquidityShock:
		builder.appendCausalLiquidityShock()
	case FixtureCausalCausalNoise:
		builder.appendCausalCausalNoise()
	case FixtureLeadlagAnchorStall:
		builder.appendLeadlagAnchorStall()
	case FixtureLeadlagInefficientLag:
		builder.appendLeadlagInefficientLag()
	case FixtureLeadlagSynchronizedDrift:
		builder.appendLeadlagSynchronizedDrift()
	case FixtureLeadlagDecoupledMove:
		builder.appendLeadlagDecoupledMove()
	case FixtureCorrelationSystemicHerd:
		builder.appendCorrelationSystemicHerd()
	case FixtureCorrelationDecoupledAlpha:
		builder.appendCorrelationDecoupledAlpha()
	case FixtureCorrelationStochasticNoise:
		builder.appendCorrelationStochasticNoise()
	case FixtureCorrelationDivergentStress:
		builder.appendCorrelationDivergentStress()
	case FixtureToxicityToxicBluff:
		builder.appendToxicityToxicBluff()
	case FixtureToxicityLiquidityVacuum:
		builder.appendToxicityLiquidityVacuum()
	case FixtureToxicityHardSupport:
		builder.appendToxicityHardSupport()
	default:
		builder.AppendInstrumentCatalog()
	}
}

func (builder *CaptureBuilder) appendCVDAggressiveDrive() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.5, 100.5, 2)
	builder.AppendBookSnapshot(testSymbolPrimary, 99.5, 20, 100.5, 20)
	builder.AppendBuyTrades(testSymbolPrimary, 96, 100, 4)
}

func (builder *CaptureBuilder) appendCVDHiddenAbsorption() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.5, 100.5, 0.1)
	builder.AppendBookSnapshot(testSymbolPrimary, 99.5, 30, 100.5, 30)

	for index := range 80 {
		side := "buy"

		if index%3 == 1 {
			side = "sell"
		}

		builder.appendFrame(public.TradesChannel, "update", []market.TradeUpdate{{
			Symbol:    testSymbolPrimary,
			Side:      side,
			Price:     100 + float64(index%5)*0.01,
			Qty:       8,
			Timestamp: builder.timestamp(),
		}})
	}
}

func (builder *CaptureBuilder) appendCVDStochasticBalance() {
	builder.AppendBaselineMarket()
}

func (builder *CaptureBuilder) appendCVDVolumeStarvation() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.5, 100.5, 0)
	builder.AppendBookSnapshot(testSymbolPrimary, 99.5, 20, 100.5, 20)
	builder.AppendBuyTrades(testSymbolPrimary, 2, 100, 0.01)
}

func (builder *CaptureBuilder) appendFluidLaminar() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.9, 100.1, 0)
	builder.AppendBookSnapshot(testSymbolPrimary, 99.9, 50, 100.1, 50)
}

func (builder *CaptureBuilder) appendFluidTurbulent() {
	builder.AppendInstrumentCatalog()

	for index := range 24 {
		spread := 0.5 + float64(index%6)*0.2
		builder.AppendBookSnapshot(
			testSymbolPrimary,
			100-spread/2, 10+float64(index),
			100+spread/2, 10+float64(index%3),
		)
	}
}

func (builder *CaptureBuilder) appendFluidInertial() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 98, 102, 3)
	builder.AppendBookSnapshot(testSymbolPrimary, 98, 5, 102, 5)
	builder.AppendBuyTrades(testSymbolPrimary, 40, 100, 3)
}

func (builder *CaptureBuilder) appendFluidViscous() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.8, 100.2, 0)
	builder.AppendBookSnapshot(testSymbolPrimary, 99.8, 200, 100.2, 200)
}

func (builder *CaptureBuilder) appendHawkesFrenzy() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99, 101, 0)
	builder.AppendTradeBurst(testSymbolPrimary, 160, 100, 2.5, "buy")
}

func (builder *CaptureBuilder) appendHawkesSaturation() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99, 101, 0)
	builder.AppendTradeBurst(testSymbolPrimary, 256, 100, 3, "buy")
}

func (builder *CaptureBuilder) appendHawkesOrganic() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.5, 100.5, 0)

	for index := range 12 {
		builder.appendFrame(public.TradesChannel, "update", []market.TradeUpdate{{
			Symbol:    testSymbolPrimary,
			Side:      "buy",
			Price:     100,
			Qty:       0.5,
			Timestamp: builder.timestamp().Add(time.Duration(index) * time.Second),
		}})
	}
}

func (builder *CaptureBuilder) appendHawkesExhaustion() {
	builder.appendHawkesSaturation()
	builder.tick += 40
}

func (builder *CaptureBuilder) appendDepthflowLoadedImbalance() {
	builder.AppendDepthflowTape(testSymbolPrimary)
}

func (builder *CaptureBuilder) appendDepthflowSpoofTrap() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.5, 100.5, 0)
	builder.AppendBookSnapshot(testSymbolPrimary, 99.5, 5000, 100.5, 1)
}

func (builder *CaptureBuilder) appendDepthflowBookThinning() {
	builder.AppendInstrumentCatalog()
	builder.AppendBookThinning(testSymbolPrimary, 16)
}

func (builder *CaptureBuilder) appendDepthflowDenseNeutrality() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.9, 100.1, 0)
	builder.AppendBookSnapshot(testSymbolPrimary, 99.9, 40, 100.1, 40)
}

func (builder *CaptureBuilder) appendSentimentSystemicSlump() {
	builder.AppendInstrumentCatalog()
	builder.AppendSentimentSlumpCrossSection()
}

func (builder *CaptureBuilder) appendSentimentRiskOnSurge() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolLeader, 120, 119, 121, 8)
	builder.AppendTicker(testSymbolSecondary, 110, 109, 111, 6)
	builder.AppendTicker(testSymbolPrimary, 105, 104, 106, 5)
}

func (builder *CaptureBuilder) appendSentimentDivergentMove() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolLeader, 100, 99, 101, -6)
	builder.AppendTicker(testSymbolSecondary, 90, 89, 91, -2)
	builder.AppendTicker(testSymbolPrimary, 95, 94, 96, 4)
}

func (builder *CaptureBuilder) appendLiquidityRobust() {
	builder.AppendInstrumentCatalog()
	builder.AppendLiquidityCrossSection()
}

func (builder *CaptureBuilder) appendLiquidityMedianDepth() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 10, 9.9, 10.1, 1)
	builder.AppendTicker(testSymbolSecondary, 10, 9.9, 10.1, 1)
	builder.AppendTicker(testSymbolLeader, 10, 9.9, 10.1, 1)
}

func (builder *CaptureBuilder) appendLiquidityExtremeScarcity() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 10, 9.9, 10.1, 0.1)
	builder.AppendTicker(testSymbolSecondary, 50, 49.9, 50.1, 2)
	builder.AppendTicker(testSymbolLeader, 80, 79.9, 80.1, 3)
}

func (builder *CaptureBuilder) appendPumpdumpVerticalIgnition() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 10, 9.9, 10.1, 2)
	builder.AppendPumpLift(testSymbolPrimary, 40)
}

func (builder *CaptureBuilder) appendPumpdumpCoiledCompression() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 10, 9.99, 10.01, 0)

	for index := range 20 {
		builder.appendFrame(public.TradesChannel, "update", []market.TradeUpdate{{
			Symbol:    testSymbolPrimary,
			Side:      "buy",
			Price:     10 + float64(index)*0.001,
			Qty:       0.2,
			Timestamp: builder.timestamp(),
		}})
	}
}

func (builder *CaptureBuilder) appendPumpdumpOrganicTrend() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 10, 9.9, 10.1, 0.5)

	for index := range 24 {
		builder.appendFrame(public.TradesChannel, "update", []market.TradeUpdate{{
			Symbol:    testSymbolPrimary,
			Side:      "buy",
			Price:     10 + float64(index)*0.02,
			Qty:       0.8,
			Timestamp: builder.timestamp(),
		}})
	}
}

func (builder *CaptureBuilder) appendPumpdumpFadedExhaustion() {
	builder.appendPumpdumpVerticalIgnition()

	for index := range 16 {
		builder.appendFrame(public.TradesChannel, "update", []market.TradeUpdate{{
			Symbol:    testSymbolPrimary,
			Side:      "sell",
			Price:     11 - float64(index)*0.05,
			Qty:       1.2,
			Timestamp: builder.timestamp(),
		}})
	}
}

func (builder *CaptureBuilder) appendExhaustMechanicalCollapse() {
	builder.AppendInstrumentCatalog()
	builder.AppendBookThinning(testSymbolPrimary, 16)
}

func (builder *CaptureBuilder) appendExhaustFragileExpansion() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 95, 105, 0)

	for index := range 14 {
		widen := float64(index) * 0.4
		builder.AppendBookSnapshot(testSymbolPrimary, 100-widen, 8, 100+widen, 8)
	}
}

func (builder *CaptureBuilder) appendExhaustThermalExhaustion() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.5, 100.5, 0)
	builder.AppendBuyTrades(testSymbolPrimary, 24, 100, 2)
	builder.tick += 8
}

func (builder *CaptureBuilder) appendExhaustActiveReversal() {
	builder.AppendInstrumentCatalog()
	builder.AppendBuyTrades(testSymbolPrimary, 20, 100, 2)
	builder.AppendSellTrade(testSymbolPrimary, 99.5, 30)
}

func (builder *CaptureBuilder) appendCausalEndogenousAlpha() {
	builder.AppendCausalCrossSection()
}

func (builder *CaptureBuilder) appendCausalSystemicBeta() {
	builder.AppendCausalCrossSection()
}

func (builder *CaptureBuilder) appendCausalLiquidityShock() {
	builder.AppendBlackSwanCrash()
}

func (builder *CaptureBuilder) appendCausalCausalNoise() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.5, 100.5, 0)
	builder.AppendBuyTrades(testSymbolPrimary, 18, 100, 2)
	builder.AppendTicker(testSymbolPrimary, 100, 99.5, 100.5, 0)
}

func (builder *CaptureBuilder) appendLeadlagAnchorStall() {
	builder.AppendInstrumentCatalog()
	builder.AppendLeadLagStall()
}

func (builder *CaptureBuilder) appendLeadlagInefficientLag() {
	builder.AppendInstrumentCatalog()
	builder.AppendLeadLagStall()
}

func (builder *CaptureBuilder) appendLeadlagSynchronizedDrift() {
	builder.AppendInstrumentCatalog()
	builder.AppendLeadLagStall()
}

func (builder *CaptureBuilder) appendLeadlagDecoupledMove() {
	builder.AppendInstrumentCatalog()
	builder.AppendLeadLagStall()
}

func (builder *CaptureBuilder) appendCorrelationSystemicHerd() {
	builder.AppendInstrumentCatalog()
	builder.AppendCorrelationHerd()
}

func (builder *CaptureBuilder) appendCorrelationDecoupledAlpha() {
	builder.AppendInstrumentCatalog()
}

func (builder *CaptureBuilder) appendCorrelationStochasticNoise() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.5, 100.5, 0)
}

func (builder *CaptureBuilder) appendCorrelationDivergentStress() {
	builder.AppendInstrumentCatalog()
}

func (builder *CaptureBuilder) appendToxicityToxicBluff() {
	builder.AppendInstrumentCatalog()
	builder.AppendToxicityCancelWall(testSymbolPrimary, 100)
}

func (builder *CaptureBuilder) appendToxicityLiquidityVacuum() {
	builder.AppendInstrumentCatalog()
	builder.AppendToxicityCancelWall(testSymbolPrimary, 100)
	builder.AppendBookSnapshot(testSymbolPrimary, 99.5, 0.0001, 100.5, 0.0001)
}

func (builder *CaptureBuilder) appendToxicityHardSupport() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.5, 100.5, 0)
	builder.AppendBookSnapshot(testSymbolPrimary, 99.5, 80, 100.5, 80)
}
