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
	SourceDesk        SourceType = "desk"
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

var SignalSourceStrings = []string{
	"correlation",
	"cvd",
	"depthflow",
	"exhaustion",
	"hawkes",
	"leadlag",
	"liquidity",
	"pumpdump",
	"sentiment",
	"toxicity",
}

var LogicSourceStrings = []string{
	"category",
	"manifold",
	"resonance",
	"graph",
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

var TickerReceiverStrings = []string{
	"correlation",
	"cvd",
	"leadlag",
	"liquidity",
	"pumpdump",
	"sentiment",
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

var TradeReceiverStrings = []string{
	"cvd",
	"depthflow",
	"exhaustion",
	"hawkes",
	"pumpdump",
	"toxicity",
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
	SourcePumpDump,
}

var Level3ReceiverStrings = []string{
	"toxicity",
	"pumpdump",
}

/*
AcceptedBookReceivers names every signal whose state changes after an accepted
Level 3 frame has already been committed to the authoritative local book.
*/
var AcceptedBookReceivers = append(
	append([]SourceType(nil), BookReceivers...),
	Level3Receivers...,
)
