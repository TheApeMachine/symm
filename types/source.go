package types

type SourceType string

const (
	SourceCorrelation SourceType = "correlation"
	SourceDepthFlow   SourceType = "depthflow"
	SourceHawkes     SourceType = "hawkes"
	SourceLiquidity  SourceType = "liquidity"
	SourcePumpDump   SourceType = "pumpdump"
	SourceSentiment  SourceType = "sentiment"
	SourceToxicity   SourceType = "toxicity"
)

var SignalSources = []SourceType{
	SourceCorrelation,
	SourceDepthFlow,
	SourceHawkes,
	SourceLiquidity,
	SourcePumpDump,
	SourceSentiment,
	SourceToxicity,
}

var SignalSourceStrings = []string{
	"correlation",
	"depthflow",
	"hawkes",
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

var TradeReceivers = []SourceType{
	SourceCorrelation,
	SourceHawkes,
	SourcePumpDump,
	SourceSentiment,
}

var TradeReceiverStrings = []string{
	"correlation",
	"hawkes",
	"pumpdump",
	"sentiment",
}

var BookReceivers = []SourceType{
	SourceDepthFlow,
	SourceLiquidity,
}

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

var AcceptedBookReceivers = append(
	append([]SourceType(nil), BookReceivers...),
	Level3Receivers...,
)
