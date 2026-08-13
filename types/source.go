package types

type SourceType string

const (
	SourceCorrelation SourceType = "correlation"
	SourceCVD         SourceType = "cvd"
	SourceDepthFlow   SourceType = "depthflow"
	SourceExhaustion  SourceType = "exhaustion"
	SourceHawkes      SourceType = "hawkes"
	SourceLeadLag     SourceType = "leadlag"
	SourceLiquidity   SourceType = "liquidity"
	SourcePumpDump    SourceType = "pumpdump"
	SourceCategory    SourceType = "category"
	SourceSentiment   SourceType = "sentiment"
	SourceToxicity    SourceType = "toxicity"
	SourceAnalyzer    SourceType = "analyzer"
	SourceManifold    SourceType = "manifold"
	SourceResonance   SourceType = "resonance"
	SourceCausal      SourceType = "causal"
	SourceCognition   SourceType = "cognition"
	SourceGraph       SourceType = "graph"
	SourceAllocator   SourceType = "allocator"
	SourceArbiter     SourceType = "arbiter"
	SourceEvaluator   SourceType = "evaluator"
	SourcePlanner     SourceType = "planner"
	SourceTrader      SourceType = "trader"
	SourceEquity      SourceType = "equity"
	SourceRegulator   SourceType = "regulator"
)

/*
SignalSources is the complete configured measurement source set used by signal
scheduling and cross-source inspection.
*/
var SignalSources = []SourceType{
	SourceCorrelation,
	SourceCVD,
	SourceDepthFlow,
	SourceExhaustion,
	SourceHawkes,
	SourceLeadLag,
	SourceLiquidity,
	SourcePumpDump,
	SourceSentiment,
	SourceToxicity,
}

/*
TickerReceivers names the signals that drain per-symbol ticker queues.
*/
var TickerReceivers = []SourceType{
	SourceCorrelation,
	SourceCVD,
	SourceLeadLag,
	SourceLiquidity,
	SourcePumpDump,
	SourceSentiment,
}

/*
TradeReceivers names the signals that drain per-symbol trade queues.
*/
var TradeReceivers = []SourceType{
	SourceCVD,
	SourceDepthFlow,
	SourceExhaustion,
	SourceHawkes,
	SourcePumpDump,
	SourceToxicity,
}

/*
BookReceivers names the signals whose inputs change when the authoritative
Level 3 manager applies an order update.
*/
var BookReceivers = []SourceType{
	SourceDepthFlow,
	SourceExhaustion,
}

/*
Level3Receivers names the signals that consume accepted order-identity events.
*/
var Level3Receivers = []SourceType{
	SourceToxicity,
}
