package types

import (
	"time"

	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
Channel names on the system workspace bus. One Workspace is shared by the
whole pipeline; stages subscribe to the named channels they consume and
publish to the named channels they produce.
*/
const (
	ChannelTickers        = "tickers"
	ChannelTrades         = "trades"
	ChannelLevel3         = "level3"
	ChannelFuturesTickers = "futures_tickers"
	ChannelFuturesTrades  = "futures_trades"

	ChannelSignals      = "signals"
	ChannelEnvelopes    = "envelopes"
	ChannelPerspectives = "perspectives"
	ChannelDecisions    = "decisions"
	ChannelExecutions   = "executions"
	ChannelOrders       = "orders"
	ChannelPositions    = "positions"
	ChannelPnl          = "pnl"
	ChannelTelemetry    = "telemetry"
)

/*
ResonanceArtifact carries one settled predictive manifold reading to
downstream stages. The solver publishes the coder's own snapshot of the
manifold rather than the mutable model itself, so no downstream stage can
advance shared state.
*/
type ResonanceArtifact struct {
	Symbol   string
	At       time.Time
	Snapshot *learning.ManifoldReading
	Forecast *ResonanceReturnForecast
	Dynamics *telemetry.EnvelopeResonanceDynamicsT

	// Predictive-head projection data. The workspace observer projects these
	// into the dashboard ResonanceFrame, so the domain payload carries the wire
	// coordinates instead of the solver holding a ChannelUI handle.
	ForwardCurve     []float64
	ForwardRetention []float64
	SupportedHorizon int
	Calibrated       bool
	ResolvedSteps    int
	Readout          []float64
	Confidence       float64
	// LastResolutionPrediction is the direction the head actually issued at t,
	// preserved so the outcome check at t+1 scores that decision and not a
	// recomputed forecast.
	LastResolutionPrediction float64
	LastResolutionTarget     float64
	LastResolutionError      float64
	// TargetSeries records which candidate feature the head was trained to forecast.
	TargetSeries string
}
