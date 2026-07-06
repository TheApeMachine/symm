package logic

import (
	"fmt"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

type metricSchema struct {
	name string
	keys []string
}

func sourceMetricSchemas() map[types.SourceType][]metricSchema {
	return map[types.SourceType][]metricSchema{
		types.SourceHawkes: {{
			name: "hawkes_trade",
			keys: []string{"branchingRatio", "intensityRatio", "spectralRadius", "baselineMu"},
		}},
		types.SourceCVD: {{
			name: "trade_flow",
			keys: []string{"drive", "absorption", "balance", "starvation"},
		}},
		types.SourceFluid: {{
			name: "fluid_book",
			keys: []string{"reynolds", "viscosity", "divergence", "vorticity", "turbulence", "memory"},
		}},
		types.SourceDepthFlow: {{
			name: "bookflow",
			keys: []string{"loadedScore", "spoofScore", "thinScore", "neutralScore"},
		}},
		types.SourceLiquidity: {{
			name: "liquidity_ticker",
			keys: []string{"depthScore", "scarcityScore", "medianScore"},
		}},
		types.SourcePumpDump: {
			{
				name: "pumpdump_ticker",
				keys: []string{"ignition", "compression", "trend", "exhaustion", "rvol", "spread"},
			},
			{
				name: "pumpdump_trade",
				keys: []string{"drive", "absorption", "balance", "starvation"},
			},
			{
				name: "pumpdump_book",
				keys: []string{"loadedScore", "spoofScore", "thinScore", "neutralScore"},
			},
		},
		types.SourceExhaustion: {{
			name: "exhaustion",
			keys: []string{"fragile", "reversal", "mechanical", "thermal", "urgency"},
		}},
		types.SourceToxicity: {{
			name: "toxicity",
			keys: []string{"supportScore", "bluffScore", "vacuumScore"},
		}},
		types.SourceLeadLag: {{
			name: "leadlag_ticker",
			keys: []string{"sampleSupport", "lagFraction", "stall", "decoupled", "inefficient"},
		}},
		types.SourceCorrelation: {{
			name: "correlation_ticker",
			keys: []string{"relativeEnergy", "signed", "noiseScore", "alphaScore", "herdScore", "stressScore"},
		}},
		types.SourceSentiment: {{
			name: "sentiment_ticker",
			keys: []string{"breadth", "leaderEvidence", "leaderStrength", "slumpScore", "divergentScore", "surgeScore"},
		}},
	}
}

func (schema metricSchema) Matches(measurement *types.Measurement) bool {
	return schema.firstMissing(measurement) == ""
}

func (schema metricSchema) Error(measurement *types.Measurement) error {
	missing := schema.firstMissing(measurement)
	if missing == "" {
		return nil
	}

	return errnie.Error(errnie.Err(
		errnie.Validation,
		fmt.Sprintf(
			"decision boundary: %s metric %q required",
			schema.name,
			missing,
		),
		nil,
	))
}

func (schema metricSchema) firstMissing(
	measurement *types.Measurement,
) string {
	if measurement == nil || measurement.Metrics == nil {
		return schema.keys[0]
	}

	for _, key := range schema.keys {
		if _, ok := measurement.Metrics[key]; !ok {
			return key
		}
	}

	return ""
}
