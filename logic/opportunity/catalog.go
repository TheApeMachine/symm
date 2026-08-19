/*
Package opportunity defines the identifiable, actable opportunity archetypes
the strategy reasons over, and scores the current observations against them.

The category solver classifies microstructure texture; this package answers
"which opportunity is this, which lifecycle phase is it in, and is the evidence
trustworthy enough to act?" Every archetype is a conjunction of conditioned
observations across the pipeline, attenuated by each leg's epistemic trust.
*/
package opportunity

import "github.com/theapemachine/symm/types"

/*
Catalog is the authoritative opportunity taxonomy. Order matters only for
deterministic ties; scoring consults every archetype, not just the first match.
*/
var Catalog = []types.OpportunityArchetype{
	{
		Type: types.OpportunitySuddenPump,
		Precursors: []types.CategoryType{
			types.VerticalIgnition,
			types.RiskOnSurge,
		},
		Supports: []types.ObservationCondition{
			{
				Source:          types.SourcePumpDump,
				Metric:          string(types.MetricRVOL),
				Name:            "rvol surge",
				State:           "rvol above its own recent median while lift stays positive",
				Supports:        true,
				MaturityFloor:   0.4,
				SeparationFloor: 0.4,
			},
			{
				Source:          types.SourceHawkes,
				Metric:          string(types.MetricSpectralRadius),
				Name:            "near-critical cascade",
				State:           "spectral radius approaching one with positive branching",
				Supports:        true,
				MaturityFloor:   0.4,
				SeparationFloor: 0.4,
			},
			{
				Source:          types.SourceDepthFlow,
				Metric:          string(types.MetricThinScore),
				Name:            "hollow ask book",
				State:           "thin ask depth below the symbol's own touch-depth median",
				Supports:        true,
				MaturityFloor:   0.3,
				SeparationFloor: 0.3,
			},
			{
				Source:          types.SourceCVD,
				Metric:          string(types.MetricDrive),
				Name:            "aggressive buy drive",
				State:           "buy aggressor flow dominant and still accelerating",
				Supports:        true,
				MaturityFloor:   0.4,
				SeparationFloor: 0.4,
			},
		},
		Opposes: []types.ObservationCondition{
			{
				Source:          types.SourceToxicity,
				Metric:          string(types.MetricBluffScore),
				Name:            "spoofed wall",
				State:           "cancelled retreating quantity marks the wall as fake",
				Contradicts:     true,
				MaturityFloor:   0.3,
				SeparationFloor: 0.3,
			},
			{
				Source:          types.SourceExhaustion,
				Metric:          string(types.MetricExhaustion),
				Side:            types.SideSell,
				Name:            "late seller exhaustion",
				State:           "the move is already decelerating into rejection",
				Contradicts:     true,
				MaturityFloor:   0.4,
				SeparationFloor: 0.3,
			},
		},
		RolloutDynamics: "pump",
	},
	{
		Type:       types.OpportunityCoiledCompression,
		Precursors: []types.CategoryType{types.CoiledCompression},
		Supports: []types.ObservationCondition{
			{
				Source:          types.SourcePumpDump,
				Metric:          string(types.MetricCompression),
				Name:            "spread compression",
				State:           "spread tightening below its own baseline",
				Supports:        true,
				MaturityFloor:   0.4,
				SeparationFloor: 0.4,
			},
			{
				Source:          types.SourceManifold,
				Metric:          "pressure_gradient",
				Name:            "coiled pressure",
				State:           "pressure gradient accumulating without divergence release",
				Supports:        true,
				MaturityFloor:   0.4,
				SeparationFloor: 0.4,
			},
			{
				Source:          types.SourceManifold,
				Metric:          "coherence",
				Name:            "oscillator phase sync",
				State:           "phase coherence rising toward synchronized release",
				Supports:        true,
				MaturityFloor:   0.4,
				SeparationFloor: 0.4,
			},
		},
		Opposes: []types.ObservationCondition{
			{
				Source:          types.SourceDepthFlow,
				Metric:          string(types.MetricNeutralScore),
				Name:            "dense neutrality",
				State:           "deep two-sided book absorbs without releasing",
				Contradicts:     true,
				MaturityFloor:   0.3,
				SeparationFloor: 0.3,
			},
		},
		RolloutDynamics: "coiled",
	},
	{
		Type:       types.OpportunityDailyRiser,
		Precursors: []types.CategoryType{types.OrganicTrend},
		Supports: []types.ObservationCondition{
			{
				Source:          types.SourceResonance,
				Metric:          "generalized_velocity",
				Name:            "latent velocity",
				State:           "positive generalized velocity with low residual",
				Supports:        true,
				MaturityFloor:   0.5,
				SeparationFloor: 0.4,
			},
			{
				Source:          types.SourceSentiment,
				Metric:          string(types.MetricBreadth),
				Name:            "positive breadth",
				State:           "cohort breadth supporting the leader",
				Supports:        true,
				MaturityFloor:   0.4,
				SeparationFloor: 0.4,
			},
			{
				Source:          types.SourceManifold,
				Metric:          "divergence",
				Name:            "laminar flow",
				State:           "low divergence across the fluid field",
				Supports:        true,
				MaturityFloor:   0.4,
				SeparationFloor: 0.3,
			},
		},
		Opposes: []types.ObservationCondition{
			{
				Source:          types.SourceExhaustion,
				Metric:          string(types.MetricFragile),
				Name:            "fragile expansion",
				State:           "the trend is built on fragile marginal volume",
				Contradicts:     true,
				MaturityFloor:   0.4,
				SeparationFloor: 0.3,
			},
		},
		RolloutDynamics: "riser",
	},
	{
		Type:       types.OpportunityInefficientLag,
		Precursors: []types.CategoryType{types.InefficientLag},
		Supports: []types.ObservationCondition{
			{
				Source:          types.SourceLeadLag,
				Metric:          string(types.MetricInefficient),
				Name:            "lag correlation",
				State:           "leader path predicts this symbol's next move",
				Supports:        true,
				MaturityFloor:   0.5,
				SeparationFloor: 0.5,
			},
			{
				Source:          types.SourceCorrelation,
				Metric:          string(types.MetricAlphaScore),
				Name:            "decoupled alpha",
				State:           "own path carries signal beyond cohort beta",
				Supports:        true,
				MaturityFloor:   0.4,
				SeparationFloor: 0.4,
			},
		},
		Opposes: []types.ObservationCondition{
			{
				Source:          types.SourceLeadLag,
				Metric:          string(types.MetricStall),
				Name:            "anchor stall",
				State:           "the presumed leader has stopped moving",
				Contradicts:     true,
				MaturityFloor:   0.4,
				SeparationFloor: 0.3,
			},
		},
		RolloutDynamics: "lag",
	},
	{
		Type:       types.OpportunityAbsorptionReversal,
		Precursors: []types.CategoryType{types.HiddenAbsorption},
		Supports: []types.ObservationCondition{
			{
				Source:          types.SourceCVD,
				Metric:          string(types.MetricAbsorption),
				Name:            "massive absorption",
				State:           "passive absorption outlasting the aggressive flow",
				Supports:        true,
				MaturityFloor:   0.4,
				SeparationFloor: 0.4,
			},
			{
				Source:          types.SourceToxicity,
				Metric:          string(types.MetricBluffScore),
				Name:            "bluff wall",
				State:           "cancellation marks the pressure as fake",
				Supports:        true,
				MaturityFloor:   0.3,
				SeparationFloor: 0.3,
			},
		},
		Opposes: []types.ObservationCondition{
			{
				Source:          types.SourceHawkes,
				Metric:          string(types.MetricSpectralRadius),
				Name:            "still-critical cascade",
				State:           "the arrival process is still self-exciting upward",
				Contradicts:     true,
				MaturityFloor:   0.3,
				SeparationFloor: 0.3,
			},
		},
		RolloutDynamics: "absorption",
	},
}
