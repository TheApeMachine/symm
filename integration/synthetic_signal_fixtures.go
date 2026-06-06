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
	builder.AppendBuyTrades(testSymbolPrimary, 24, 100, 1)
	builder.Advance(20 * time.Minute)
	builder.AppendBuyTradeRamp(testSymbolPrimary, 16, 102, 2, 20)
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
			Price:     100,
			Qty:       8,
			Timestamp: builder.timestamp(),
		}})
	}
}

func (builder *CaptureBuilder) appendCVDStochasticBalance() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.5, 100.5, 0)
	builder.AppendBookSnapshot(testSymbolPrimary, 99.5, 20, 100.5, 20)
	builder.AppendBuyTrades(testSymbolPrimary, 24, 100, 1)
	builder.Advance(20 * time.Minute)

	for index := range 18 {
		side := "buy"

		if index%2 == 1 {
			side = "sell"
		}

		builder.appendFrame(public.TradesChannel, "update", []market.TradeUpdate{{
			Symbol:    testSymbolPrimary,
			Side:      side,
			Price:     100 + float64(index%2)*0.01,
			Qty:       1,
			Timestamp: builder.timestamp(),
		}})
	}
}

func (builder *CaptureBuilder) appendCVDVolumeStarvation() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.5, 100.5, 0)
	builder.AppendBookSnapshot(testSymbolPrimary, 99.5, 20, 100.5, 20)
	builder.AppendBuyTrades(testSymbolPrimary, 24, 100, 1)
	builder.Advance(20 * time.Minute)
	builder.AppendBuyTrades(testSymbolPrimary, 1, 100, 0.01)
}

func (builder *CaptureBuilder) appendFluidLaminar() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.9, 100.1, 0)
	builder.AppendBookSnapshot(testSymbolPrimary, 99.9, 50, 100.1, 50)
}

func (builder *CaptureBuilder) appendFluidTurbulent() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.9, 100.1, 0)
	builder.AppendBookSnapshot(testSymbolPrimary, 99.9, 50, 100.1, 50)

	for index := range 16 {
		price := 80.0

		if index%2 == 1 {
			price = 125
		}

		if index == 15 {
			price = 180
		}

		builder.AppendTicker(testSymbolPrimary, price, price-0.1, price+0.1, 4)
	}
}

func (builder *CaptureBuilder) appendFluidInertial() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.9, 100.1, 3)
	builder.AppendBookSnapshot(testSymbolPrimary, 99.9, 80, 100.1, 5)
}

func (builder *CaptureBuilder) appendFluidViscous() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 50, 150, 0)
	builder.AppendBookSnapshot(testSymbolPrimary, 50, 200, 150, 200)
}

func (builder *CaptureBuilder) appendHawkesFrenzy() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99, 101, 0)
	builder.AppendTradeBurst(testSymbolPrimary, 160, 100, 2.5, "buy")
}

func (builder *CaptureBuilder) appendHawkesSaturation() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99, 101, 0)
	builder.appendHawkesTimedTrades(testSymbolPrimary, 24, 100, 0.5, "alternate", time.Second)
	builder.Advance(15 * time.Second)

	for burstIndex := range 8 {
		builder.appendHawkesTimedTrades(
			testSymbolPrimary,
			48,
			100+float64(burstIndex),
			3,
			"alternate",
			5*time.Millisecond,
		)
		builder.Advance(3 * time.Second)
	}
}

func (builder *CaptureBuilder) appendHawkesOrganic() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.5, 100.5, 0)

	for index := range 24 {
		side := "buy"

		if index%2 == 1 {
			side = "sell"
		}

		builder.appendFrame(public.TradesChannel, "update", []market.TradeUpdate{{
			Symbol:    testSymbolPrimary,
			Side:      side,
			Price:     100,
			Qty:       0.5,
			Timestamp: builder.timestamp().Add(time.Duration(index) * time.Second),
		}})
	}
}

func (builder *CaptureBuilder) appendHawkesExhaustion() {
	builder.appendHawkesSaturation()
	builder.Advance(2 * time.Minute)
	builder.AppendTicker(testSymbolPrimary, 100, 99, 101, 0)
}

func (builder *CaptureBuilder) appendHawkesTimedTrades(
	symbol string,
	count int,
	startPrice float64,
	qty float64,
	side string,
	spacing time.Duration,
) {
	start := builder.timestamp()
	trades := make([]market.TradeUpdate, 0, count)

	for index := range count {
		tradeSide := side

		if side == "alternate" {
			tradeSide = "buy"

			if index%2 == 1 {
				tradeSide = "sell"
			}
		}

		trades = append(trades, market.TradeUpdate{
			Symbol:    symbol,
			Side:      tradeSide,
			Price:     startPrice + float64(index)*0.01,
			Qty:       qty + float64(index%5)*0.1,
			OrdType:   "market",
			TradeID:   int64(builder.tick*10000 + index),
			Timestamp: start.Add(time.Duration(index) * spacing),
		})
	}

	builder.appendFrame(public.TradesChannel, "update", trades)
	builder.Advance(time.Duration(count) * spacing)
}

func (builder *CaptureBuilder) appendDepthflowLoadedImbalance() {
	builder.AppendDepthflowTape(testSymbolPrimary)
}

func (builder *CaptureBuilder) appendDepthflowSpoofTrap() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.5, 100.5, 0)
	builder.AppendBookSnapshotLevels(
		testSymbolPrimary,
		[]market.BookLevel{
			{Price: 99.5, Qty: 1},
			{Price: 99.4, Qty: 5000},
			{Price: 99.3, Qty: 5000},
		},
		[]market.BookLevel{
			{Price: 100.5, Qty: 10},
			{Price: 100.6, Qty: 1},
			{Price: 100.7, Qty: 1},
		},
	)
}

func (builder *CaptureBuilder) appendDepthflowBookThinning() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.9, 100.1, 0)
	builder.AppendBookSnapshotLevels(
		testSymbolPrimary,
		[]market.BookLevel{
			{Price: 99.9, Qty: 20},
		},
		[]market.BookLevel{
			{Price: 100.1, Qty: 2},
			{Price: 105, Qty: 18},
		},
	)
}

func (builder *CaptureBuilder) appendDepthflowDenseNeutrality() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.9, 100.1, 0)
	builder.AppendBookSnapshot(testSymbolPrimary, 99.9, 40, 100.1, 40)
}

func (builder *CaptureBuilder) appendSentimentSystemicSlump() {
	builder.AppendInstrumentCatalog()
	builder.AppendTickerBatch([]market.TickerUpdate{
		{Symbol: testSymbolLeader, Last: 100, Bid: 99, Ask: 101, ChangePct: -4, Volume: 1000},
		{Symbol: testSymbolSecondary, Last: 80, Bid: 79, Ask: 81, ChangePct: -3, Volume: 1000},
		{Symbol: testSymbolPrimary, Last: 70, Bid: 69, Ask: 71, ChangePct: -2.5, Volume: 1000},
	})
}

func (builder *CaptureBuilder) appendSentimentRiskOnSurge() {
	builder.AppendInstrumentCatalog()
	builder.AppendTickerBatch([]market.TickerUpdate{
		{Symbol: testSymbolLeader, Last: 120, Bid: 119, Ask: 121, ChangePct: 8, Volume: 1000},
		{Symbol: testSymbolSecondary, Last: 110, Bid: 109, Ask: 111, ChangePct: 6, Volume: 1000},
		{Symbol: testSymbolPrimary, Last: 105, Bid: 104, Ask: 106, ChangePct: 5, Volume: 1000},
	})
}

func (builder *CaptureBuilder) appendSentimentDivergentMove() {
	builder.AppendInstrumentCatalog()
	builder.AppendTickerBatch([]market.TickerUpdate{
		{Symbol: testSymbolLeader, Last: 100, Bid: 99, Ask: 101, ChangePct: -6, Volume: 1000},
		{Symbol: testSymbolSecondary, Last: 90, Bid: 89, Ask: 91, ChangePct: -2, Volume: 1000},
		{Symbol: testSymbolPrimary, Last: 95, Bid: 94, Ask: 96, ChangePct: 8, Volume: 1000},
	})
}

func (builder *CaptureBuilder) appendLiquidityRobust() {
	builder.AppendInstrumentCatalog()
	builder.AppendTickerBatch([]market.TickerUpdate{
		{Symbol: testSymbolPrimary, Last: 10, Bid: 9.9, Ask: 10.1, ChangePct: 1, Volume: 2000},
		{Symbol: testSymbolSecondary, Last: 10, Bid: 9.9, Ask: 10.1, ChangePct: 1, Volume: 500},
		{Symbol: testSymbolLeader, Last: 10, Bid: 9.9, Ask: 10.1, ChangePct: 1, Volume: 700},
	})
}

func (builder *CaptureBuilder) appendLiquidityMedianDepth() {
	builder.AppendInstrumentCatalog()
	builder.AppendTickerBatch([]market.TickerUpdate{
		{Symbol: testSymbolPrimary, Last: 10, Bid: 9.9, Ask: 10.1, ChangePct: 1, Volume: 600},
		{Symbol: testSymbolSecondary, Last: 10, Bid: 9.9, Ask: 10.1, ChangePct: 1, Volume: 500},
		{Symbol: testSymbolLeader, Last: 10, Bid: 9.9, Ask: 10.1, ChangePct: 1, Volume: 700},
	})
}

func (builder *CaptureBuilder) appendLiquidityExtremeScarcity() {
	builder.AppendInstrumentCatalog()
	builder.AppendTickerBatch([]market.TickerUpdate{
		{Symbol: testSymbolPrimary, Last: 10, Bid: 9.9, Ask: 10.1, ChangePct: 0.1, Volume: 100},
		{Symbol: testSymbolSecondary, Last: 10, Bid: 9.9, Ask: 10.1, ChangePct: 2, Volume: 500},
		{Symbol: testSymbolLeader, Last: 10, Bid: 9.9, Ask: 10.1, ChangePct: 3, Volume: 700},
	})
}

func (builder *CaptureBuilder) appendPumpdumpVerticalIgnition() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 10, 9.9, 10.1, 2)
	builder.AppendPumpLift(testSymbolPrimary, 24)
	builder.AppendTradeAt(testSymbolPrimary, "buy", 14, 40, builder.timestamp())
	builder.AppendTradeAt(testSymbolPrimary, "buy", 15, 60, builder.timestamp().Add(time.Millisecond))
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
		side := "buy"

		if index%2 == 1 {
			side = "sell"
		}

		builder.appendFrame(public.TradesChannel, "update", []market.TradeUpdate{{
			Symbol:    testSymbolPrimary,
			Side:      side,
			Price:     10,
			Qty:       0.2,
			Timestamp: builder.timestamp(),
		}})
	}
}

func (builder *CaptureBuilder) appendPumpdumpFadedExhaustion() {
	builder.appendPumpdumpVerticalIgnition()
	builder.Advance(70 * time.Second)

	for index := range 24 {
		builder.appendFrame(public.TradesChannel, "update", []market.TradeUpdate{{
			Symbol:    testSymbolPrimary,
			Side:      "sell",
			Price:     10 - float64(index)*0.001,
			Qty:       0.05,
			Timestamp: builder.timestamp(),
		}})
	}
}

func (builder *CaptureBuilder) appendExhaustMechanicalCollapse() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99, 101, 0)

	for index := range 8 {
		depth := 40.0 - float64(index)*5
		builder.AppendBookSnapshot(testSymbolPrimary, 99, depth, 101, 40)
	}
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
	builder.AppendBuyTrades(testSymbolPrimary, 12, 100, 2)
	builder.AppendSellTrades(testSymbolPrimary, 4, 99.8, 2)
}

func (builder *CaptureBuilder) appendExhaustActiveReversal() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99, 101, 0)
	builder.AppendBookSnapshot(testSymbolPrimary, 99, 40, 101, 5)
	builder.AppendBookSnapshot(testSymbolPrimary, 99, 35, 101, 6)
	builder.AppendBookSnapshot(testSymbolPrimary, 99, 5, 101, 40)
}

func (builder *CaptureBuilder) appendCausalEndogenousAlpha() {
	builder.AppendInstrumentCatalog()

	start := builder.origin.Add(time.Minute)
	primaryPrice := 100.0
	secondaryPrice := 90.0
	leaderPrice := 110.0

	for index := range 16 {
		featureAt := start.Add(time.Duration(index) * 35 * time.Second)
		macro := 0.15 + float64(index%3)*0.03
		flowQty := 60 + float64(index)*8

		builder.appendCausalTickerBatch(featureAt, primaryPrice, secondaryPrice, leaderPrice, 0, macro)
		builder.appendCausalMicrostructure(testSymbolPrimary, primaryPrice, 2+float64(index%5), featureAt)
		builder.AppendTradeAt(testSymbolPrimary, "buy", primaryPrice, flowQty, featureAt.Add(time.Millisecond))
		builder.appendCausalPeerFlow(testSymbolSecondary, secondaryPrice, featureAt, index)
		builder.appendCausalPeerFlow(testSymbolLeader, leaderPrice, featureAt, index+1)

		resolveAt := featureAt.Add(31 * time.Second)
		primaryPrice *= 1 + flowQty*0.00012 + macro*0.001
		secondaryPrice *= 1 + macro*0.0001
		leaderPrice *= 1 + macro*0.0001

		builder.appendCausalTickerBatch(resolveAt, primaryPrice, secondaryPrice, leaderPrice, 0, macro)
		builder.appendCausalMicrostructure(testSymbolPrimary, primaryPrice, 2.5+float64(index%4), resolveAt)
		builder.AppendTradeAt(testSymbolPrimary, "buy", primaryPrice, flowQty, resolveAt.Add(time.Millisecond))
	}
}

func (builder *CaptureBuilder) appendCausalSystemicBeta() {
	builder.AppendCausalCrossSection()
}

func (builder *CaptureBuilder) appendCausalTickerBatch(
	at time.Time,
	primaryPrice float64,
	secondaryPrice float64,
	leaderPrice float64,
	primaryChange float64,
	peerChange float64,
) {
	timestamp := at.UTC().Format(time.RFC3339Nano)

	builder.AppendTickerBatch([]market.TickerUpdate{
		{
			Symbol: testSymbolPrimary, Last: primaryPrice,
			Bid: primaryPrice - 0.05, Ask: primaryPrice + 0.05,
			ChangePct: primaryChange, Volume: 2000, Timestamp: timestamp,
		},
		{
			Symbol: testSymbolSecondary, Last: secondaryPrice,
			Bid: secondaryPrice - 0.05, Ask: secondaryPrice + 0.05,
			ChangePct: peerChange, Volume: 1000, Timestamp: timestamp,
		},
		{
			Symbol: testSymbolLeader, Last: leaderPrice,
			Bid: leaderPrice - 0.05, Ask: leaderPrice + 0.05,
			ChangePct: peerChange, Volume: 1000, Timestamp: timestamp,
		},
	})
}

func (builder *CaptureBuilder) appendCausalMicrostructure(
	symbol string,
	price float64,
	spreadBPS float64,
	at time.Time,
) {
	spread := price * spreadBPS / 10000

	builder.AppendBookSnapshotAt(
		symbol,
		price-spread/2,
		8,
		price+spread/2,
		6,
		at,
	)
}

func (builder *CaptureBuilder) appendCausalPeerFlow(
	symbol string,
	price float64,
	at time.Time,
	index int,
) {
	side := "buy"

	if index%2 == 1 {
		side = "sell"
	}

	builder.appendCausalMicrostructure(symbol, price, 4+float64(index%3), at)
	builder.AppendTradeAt(symbol, side, price, 6, at.Add(2*time.Millisecond))
}

func (builder *CaptureBuilder) appendCausalLiquidityShock() {
	builder.AppendInstrumentCatalog()

	start := builder.origin.Add(time.Minute)
	primaryPrice := 100.0
	secondaryPrice := 90.0
	leaderPrice := 110.0

	for index := range 18 {
		featureAt := start.Add(time.Duration(index) * 35 * time.Second)
		flowQty := 70 + float64(index)*9

		builder.appendCausalTickerBatch(featureAt, primaryPrice, secondaryPrice, leaderPrice, -0.1, -0.1)
		builder.appendCausalMicrostructure(testSymbolPrimary, primaryPrice, 2, featureAt)
		builder.AppendTradeAt(testSymbolPrimary, "buy", primaryPrice, flowQty, featureAt.Add(time.Millisecond))
		builder.appendCausalPeerFlow(testSymbolSecondary, secondaryPrice, featureAt, index)
		builder.appendCausalPeerFlow(testSymbolLeader, leaderPrice, featureAt, index+1)

		resolveAt := featureAt.Add(31 * time.Second)
		primaryPrice *= 1 + flowQty*0.00014
		secondaryPrice *= 0.999
		leaderPrice *= 0.999

		builder.appendCausalTickerBatch(resolveAt, primaryPrice, secondaryPrice, leaderPrice, -0.2, -0.2)
		builder.appendCausalMicrostructure(testSymbolPrimary, primaryPrice, 2, resolveAt)
		builder.AppendTradeAt(testSymbolPrimary, "buy", primaryPrice, flowQty, resolveAt.Add(time.Millisecond))
	}

}

func (builder *CaptureBuilder) appendCausalCausalNoise() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.5, 100.5, 0)
	builder.AppendBuyTrades(testSymbolPrimary, 18, 100, 2)
	builder.Advance(time.Second)
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
	builder.AppendTicker(testSymbolPrimary, 100, 99.5, 100.5, 0)
	builder.AppendLevel3NearTouchChurn(testSymbolPrimary, 99.5)
	builder.AppendTicker(testSymbolPrimary, 100, 99.5, 100.5, 0)
}

func (builder *CaptureBuilder) appendToxicityLiquidityVacuum() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.5, 100.5, 0)
	builder.AppendBookSnapshot(testSymbolPrimary, 80, 10, 120, 10)
	builder.AppendSellTrade(testSymbolPrimary, 80, 10)
	builder.AppendBookLevelShrink(testSymbolPrimary, 80, 10, 0)
	builder.AppendBookLevelShrink(testSymbolPrimary, 80, 0, 30)
	builder.AppendBookLevelShrink(testSymbolPrimary, 80, 30, 0)
}

func (builder *CaptureBuilder) appendToxicityHardSupport() {
	builder.AppendInstrumentCatalog()
	builder.AppendTicker(testSymbolPrimary, 100, 99.5, 100.5, 0)
	builder.AppendBookSnapshot(testSymbolPrimary, 99.5, 80, 100.5, 80)
}
