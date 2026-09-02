package types

import (
	"time"

	"github.com/theapemachine/symm/nomagique/learning"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
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
	ChannelFuturesBooks   = "futures_books"
	ChannelMeasurements   = "measurements"
	ChannelCategories     = "categories"
	ChannelResonance      = "resonance"
	ChannelCausal         = "causal"
	ChannelDiagnostics    = "diagnostics"
	ChannelCognition      = "cognition"
	ChannelPhase          = "phase"
	ChannelGraphs         = "graphs"
	ChannelRelations      = "relations"
	ChannelCausalState    = "causal_state"
	ChannelOpportunities  = "opportunities"
	ChannelDecisions      = "decisions"
	ChannelExecutions     = "executions"
	ChannelRegulator      = "regulator"
	ChannelHawkes         = "hawkes"
	ChannelUI             = "ui"
	ChannelFluid          = "fluid"
	ChannelCrossSection   = "cross_section"
	ChannelDisconnect     = "disconnect"
)

/*
ResonanceArtifact carries one settled predictive manifold to downstream stages.
Solver.Step exposes the manifold only to its synchronous observer, then clears
it before the artifact crosses the Workload boundary.
*/
type ResonanceArtifact struct {
	Symbol   string
	At       time.Time
	Manifold *learning.ResonanceManifold
	Forecast *ResonanceReturnForecast
	Dynamics nmtypes.Frame

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
	// as distinct from the target it was scored against. Without it a consumer
	// can only show the realized move twice over and never the call that was
	// made.
	LastResolutionPrediction float64
	LastResolutionTarget     float64
	LastResolutionError      float64
}

/*
CausalOutput carries one symbol's causal reading to the graph stage.
*/
// NOTE: removed — the Pearl-ladder CausalOutput was a UI-only telemetry type
// that duplicated the real causal engine (logic/causal/model.go CausalModel).
// No other stage consumed it, so it was removed rather than left as a
// publish-only surface. Field spacing is intentionally preserved.
