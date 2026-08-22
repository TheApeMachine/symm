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
	SourceDerivatives SourceType = "derivatives"
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
	SourceAudit       SourceType = "audit"
)

const (
	TickerConsumerCorrelation = iota
	TickerConsumerLeadLag
	TickerConsumerLiquidity
	TickerConsumerPumpDump
	TickerConsumerSentiment
	TickerConsumerResonance
	TickerConsumerDesk
)

const (
	FuturesTickerConsumerDerivatives = iota
)

const (
	TradeConsumerCVD = iota
	TradeConsumerExhaustion
	TradeConsumerHawkes
	TradeConsumerPumpDump
)

const (
	FuturesTradeConsumerDerivatives = iota
)

const (
	Level3ConsumerDepthFlow = iota
	Level3ConsumerToxicity
	Level3ConsumerPumpDump
)

const (
	FuturesBookConsumerDerivatives = iota
)

const (
	ExecutionConsumerDesk = iota
)

const (
	MeasurementConsumerCategory = iota
	MeasurementConsumerManifold
	MeasurementConsumerGraph
	MeasurementConsumerAudit
)

const (
	CategoryConsumerGraph = iota
	CategoryConsumerCognition
	CategoryConsumerAudit
)

const (
	ResonanceConsumerCausal = iota
	ResonanceConsumerGraph
	ResonanceConsumerAudit
)

const (
	CausalConsumerGraph = iota
	CausalConsumerCausal
	CausalConsumerAudit
)

const (
	GraphConsumerPlanner = iota
	GraphConsumerAudit
)

const (
	StateConsumerStage = iota
	StateConsumerAudit
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
	SourceDerivatives,
}

/*
WorkerSources names the independently scheduled readers that own Run loops.
Their ready queues carry symbols whose MapReduce input changed.
*/
var WorkerSources = append(append([]SourceType(nil), SignalSources...),
	SourceCategory,
	SourceManifold,
	SourceCausal,
	SourceCognition,
	SourceGraph,
	SourceResonance,
	SourcePlanner,
	SourceDesk,
)

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
	"derivatives",
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
	SourceLeadLag,
	SourceLiquidity,
	SourcePumpDump,
	SourceSentiment,
}

var TickerReceiverStrings = []string{
	"correlation",
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
	SourceExhaustion,
	SourceHawkes,
	SourcePumpDump,
}

var TradeReceiverStrings = []string{
	"cvd",
	"exhaustion",
	"hawkes",
	"pumpdump",
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
	SourceDepthFlow,
	SourceToxicity,
	SourcePumpDump,
}

var Level3ReceiverStrings = []string{
	"depthflow",
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
