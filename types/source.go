package types

type SourceType string

const (
	SourceCorrelation SourceType = "correlation"
	SourceCVD         SourceType = "cvd"
	SourceDepthFlow   SourceType = "depthflow"
	SourceExhaustion  SourceType = "exhaustion"
	SourceFluid       SourceType = "fluid"
	SourceHawkes      SourceType = "hawkes"
	SourceLeadLag     SourceType = "leadlag"
	SourceLiquidity   SourceType = "liquidity"
	SourcePumpDump    SourceType = "pumpdump"
	SourceCausal      SourceType = "causal"
	SourceCategory    SourceType = "category"
	SourceResonance   SourceType = "resonance"
	SourceSentiment   SourceType = "sentiment"
	SourceToxicity    SourceType = "toxicity"
)
