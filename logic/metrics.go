package logic

import (
	"github.com/theapemachine/symm/types"
)

/*
analyzerMetrics maps each source and producer to the auxiliary metrics that
drive a deposit's free spatial axis (cellX) and momentum (momX/Y/Z). The
category (Y) and source (Z) axes are static indices, and rho/eInt come from
the classifier's per-category confidence/surprisal/strength, so no metric is
mapped for them here.
*/
var analyzerMetrics = map[types.SourceType]map[string]map[string]string{
	types.SourceHawkes: {
		"trades": {
			"cellX": "branchingRatio",
			"momX":  "intensityRatio",
			"momY":  "spectralRadius",
			"momZ":  "stationarityMargin",
		},
	},
	types.SourceFluid: {
		"book": {
			"cellX": "reynolds",
			"momX":  "sourceBalance",
			"momY":  "midAddRate",
			"momZ":  "midExecuteRate",
		},
	},
	types.SourceLeadLag: {
		"ticker": {
			"cellX": "correlation",
			"momX":  "lagFraction",
			"momY":  "sampleSupport",
			"momZ":  "stall",
		},
	},
	types.SourceSentiment: {
		"ticker": {
			"cellX": "breadth",
			"momX":  "leaderStrength",
			"momY":  "leaderEvidence",
			"momZ":  "slumpScore",
		},
	},
	types.SourcePumpDump: {
		"ticker": {
			"cellX": "rvol",
			"momX":  "spread",
			"momY":  "precursor",
			"momZ":  "compression",
		},
		"trades": {
			"cellX": "strength",
			"momX":  "net",
			"momY":  "netFraction",
			"momZ":  "net",
		},
		"book": {
			"cellX": "value",
			"momX":  "loadedScore",
			"momY":  "spoofScore",
			"momZ":  "thinScore",
		},
	},
	types.SourceExhaustion: {
		"book": {
			"cellX": "value",
			"momX":  "urgency",
			"momY":  "reversal",
			"momZ":  "fragile",
		},
		"trades": {
			"cellX": "value",
			"momX":  "urgency",
			"momY":  "reversal",
			"momZ":  "fragile",
		},
	},
	types.SourceCVD: {
		"trades": {
			"cellX": "strength",
			"momX":  "net",
			"momY":  "netFraction",
			"momZ":  "net",
		},
	},
	types.SourceDepthFlow: {
		"trades": {
			"cellX": "value",
			"momX":  "loadedScore",
			"momY":  "spoofScore",
			"momZ":  "thinScore",
		},
		"book": {
			"cellX": "value",
			"momX":  "loadedScore",
			"momY":  "spoofScore",
			"momZ":  "thinScore",
		},
	},
	types.SourceToxicity: {
		"level3": {
			"cellX": "value",
			"momX":  "bluffScore",
			"momY":  "vacuumScore",
			"momZ":  "supportScore",
		},
		"trades": {
			"cellX": "value",
			"momX":  "bluffScore",
			"momY":  "vacuumScore",
			"momZ":  "supportScore",
		},
	},
	types.SourceLiquidity: {
		"ticker": {
			"cellX": "rvol",
			"momX":  "volume",
			"momY":  "median",
			"momZ":  "depthScore",
		},
	},
	types.SourceCorrelation: {
		"ticker": {
			"cellX": "correlation",
			"momX":  "signed",
			"momY":  "relativeEnergy",
			"momZ":  "peakScore",
		},
	},
	types.SourceOHLC: {
		"ohlc": {
			"cellX": "close_location",
			"momX":  "vwap_distance",
			"momY":  "drive",
			"momZ":  "reversal",
		},
	},
}

var analyzerSources = []types.SourceType{
	types.SourceCorrelation,
	types.SourceCVD,
	types.SourceDepthFlow,
	types.SourceExhaustion,
	types.SourceFluid,
	types.SourceHawkes,
	types.SourceLeadLag,
	types.SourceLiquidity,
	types.SourcePumpDump,
	types.SourceSentiment,
	types.SourceOHLC,
	types.SourceToxicity,
}
