package types

type SourceType string

const (
	SourceCorrelation SourceType = "correlation"
	SourceCVD         SourceType = "cvd"
	SourceDepthFlow   SourceType = "depthflow"
	SourceHawkes      SourceType = "hawkes"
	SourceLeadLag     SourceType = "leadlag"
	SourceLiquidity   SourceType = "liquidity"
	SourceMorphology  SourceType = "morphology"
	SourcePumpDump    SourceType = "pumpdump"
	SourceSentiment   SourceType = "sentiment"
	SourceToxicity    SourceType = "toxicity"
)

var SignalSources = []SourceType{
	SourceCorrelation,
	SourceCVD,
	SourceDepthFlow,
	SourceHawkes,
	SourceLeadLag,
	SourceLiquidity,
	SourceMorphology,
	SourcePumpDump,
	SourceSentiment,
	SourceToxicity,
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

var TradeReceivers = []SourceType{
	SourceCorrelation,
	SourceCVD,
	SourceHawkes,
	SourceLeadLag,
	SourcePumpDump,
	SourceSentiment,
}

var TradeReceiverStrings = []string{
	"correlation",
	"cvd",
	"hawkes",
	"leadlag",
	"pumpdump",
	"sentiment",
}

var BookReceivers = []SourceType{
	SourceDepthFlow,
	SourceLiquidity,
}

var Level3Receivers = []SourceType{
	SourceDepthFlow,
	SourceMorphology,
	SourceToxicity,
	SourcePumpDump,
}

var Level3ReceiverStrings = []string{
	"depthflow",
	"morphology",
	"toxicity",
	"pumpdump",
}

var AcceptedBookReceivers = append(
	append([]SourceType(nil), BookReceivers...),
	Level3Receivers...,
)
