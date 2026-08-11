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

var TickerReceivers = []SourceType{
	SourceCorrelation,
	SourceCVD,
	SourceLeadLag,
	SourceLiquidity,
	SourceSentiment,
	SourceToxicity,
}

var TradeReceivers = []SourceType{
	SourceCVD,
	SourceDepthFlow,
	SourceExhaustion,
	SourceHawkes,
	SourcePumpDump,
	SourceToxicity,
}
