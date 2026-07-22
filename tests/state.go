package tests

import (
	"time"

	marketsignal "github.com/theapemachine/symm/tests/fixtures/signal"
)

/*
MarketState names fixture regimes while MarketAction and MarketStep expose
economic inputs without requiring tests to import an individual fixture.
*/
type MarketState = marketsignal.State
type MarketAction = marketsignal.Action
type MarketStep = marketsignal.Step

const (
	MarketStateBaseline          = marketsignal.Baseline
	MarketStateFastPump          = marketsignal.FastPump
	MarketStateSlowPump          = marketsignal.SlowPump
	MarketStateFastDump          = marketsignal.FastDump
	MarketStateSlowDump          = marketsignal.SlowDump
	MarketStateVolumeAbsorption  = marketsignal.VolumeAbsorption
	MarketStateLowVolumeLift     = marketsignal.LowVolumeLift
	MarketStateSpreadCompression = marketsignal.SpreadCompression
	MarketStateThinLiquidity     = marketsignal.ThinLiquidity
	MarketStateLoadedLiquidity   = marketsignal.LoadedLiquidity
	MarketStateLiquidityRetreat  = marketsignal.LiquidityRetreat
	MarketStateSpoofLiquidity    = marketsignal.SpoofLiquidity
	MarketStateDepthThinning     = marketsignal.DepthThinning
	MarketStateSlowCadenceLift   = marketsignal.SlowCadenceLift
	MarketStateSmallLift         = marketsignal.SmallDisplacementLift
	MarketStateSpreadControl     = marketsignal.SpreadControl
	MarketStateLeaderFollower    = marketsignal.LeaderFollower
	MarketStateAdverseDivergence = marketsignal.AdverseDivergence
	MarketMoveMid                = marketsignal.MoveMid
	MarketTrade                  = marketsignal.Trade
	MarketAdd                    = marketsignal.Add
	MarketCancel                 = marketsignal.Cancel
	MarketRefill                 = marketsignal.Refill
	MarketWidenSpread            = marketsignal.WidenSpread
	MarketTightenSpread          = marketsignal.TightenSpread
)

/*
MarketOptions supplies the deterministic start time shared by every wire feed.
*/
type MarketOptions struct {
	Start time.Time
}

var defaultMarketStart = time.Date(2026, 7, 21, 9, 0, 0, 0, time.UTC)
