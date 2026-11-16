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
	"resonance",
}

var SourceStrings = []string{
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
	"resonance",
	"regions",
	"paths",
	"decisions",
	"trade",
}
