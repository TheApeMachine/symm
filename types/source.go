package types

type SourceType string

const (
	SourceCorrelation SourceType = "correlation"
	SourceCVD         SourceType = "cvd"
	SourceDepthFlow   SourceType = "depthflow"
	SourceHawkes      SourceType = "hawkes"
	SourceLeadLag     SourceType = "leadlag"
	SourceLiquidity   SourceType = "liquidity"
	SourcePumpDump    SourceType = "pumpdump"
	SourceSentiment   SourceType = "sentiment"
	SourceToxicity    SourceType = "toxicity"
	SourceMorphology  SourceType = "morphology"
)

/*
SignalSources is the complete configured measurement source set used by signal
scheduling and cross-source inspection.
*/
var SignalSources = []SourceType{
	SourceCorrelation,
	SourceCVD,
	SourceDepthFlow,
	SourceHawkes,
	SourceLeadLag,
	SourceLiquidity,
	SourcePumpDump,
	SourceSentiment,
	SourceToxicity,
	SourceMorphology,
}

var SignalSourceStrings = []string{
	"correlation",
	"cvd",
	"depthflow",
	"hawkes",
	"leadlag",
	"liquidity",
	"morphology",
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
TradeReceivers names the signals that drain per-symbol trade queues.
*/
var TradeReceivers = []SourceType{
	SourceCVD,
	SourceHawkes,
	SourcePumpDump,
}

var TradeReceiverStrings = []string{
	"cvd",
	"hawkes",
	"pumpdump",
}

/*
BookReceivers names the signals whose inputs change when the authoritative
Level 3 manager applies an order update.
*/
var BookReceivers = []SourceType{
	SourceDepthFlow,
}

/*
Level3Receivers names the signals that consume accepted order-identity events.
*/
var Level3Receivers = []SourceType{
	SourceDepthFlow,
	SourceToxicity,
	SourcePumpDump,
	SourceMorphology,
}

var Level3ReceiverStrings = []string{
	"depthflow",
	"toxicity",
	"pumpdump",
	"morphology",
}

/*
AcceptedBookReceivers names every signal whose state changes after an accepted
Level 3 frame has already been committed to the authoritative local book.
*/
var AcceptedBookReceivers = append(
	append([]SourceType(nil), BookReceivers...),
	Level3Receivers...,
)
