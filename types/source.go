package types

type SourceType string

const (
	SourceCausal      SourceType = "causal"
	SourceCorrelation SourceType = "correlation"
	SourceCVD         SourceType = "cvd"
	SourceDepthFlow   SourceType = "depthflow"
	SourceExhaustion  SourceType = "exhaustion"
	SourceFluid       SourceType = "fluid"
	SourceHawkes      SourceType = "hawkes"
	SourceLeadLag     SourceType = "leadlag"
	SourceLiquidity   SourceType = "liquidity"
	SourceManifold    SourceType = "manifold"
	SourcePumpDump    SourceType = "pumpdump"
	SourceResonance   SourceType = "resonance"
	SourceSentiment   SourceType = "sentiment"
	SourceToxicity    SourceType = "toxicity"
)
