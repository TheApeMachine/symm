package types

import (
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
	ChannelCognition      = "cognition"
	ChannelPhase          = "phase"
	ChannelGraphs         = "graphs"
	ChannelDecisions      = "decisions"
	ChannelExecutions     = "executions"
	ChannelUI             = "ui"
	ChannelFluid          = "fluid"
	ChannelCrossSection   = "cross_section"
)

/*
ResonanceArtifact carries one settled predictive manifold to the causal and
graph stages. The manifold itself does not own a symbol, so the publisher
stamps it.
*/
type ResonanceArtifact struct {
	Symbol   string
	Manifold *learning.ResonanceManifold
	Forecast *ResonanceReturnForecast
	Dynamics nmtypes.Frame
}

/*
CausalOutput carries one symbol's causal reading to the graph stage.
*/
type CausalOutput struct {
	Symbol string
	Rows   map[string]any
}
