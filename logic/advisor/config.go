package advisor

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/theapemachine/errnie"
)

/*
PredictionMoveConfig represents serializable movement directions for falsifiable predictions.
*/
type PredictionMoveConfig string

const (
	MoveNoMove   PredictionMoveConfig = "NOMOVE"
	MoveIncrease PredictionMoveConfig = "INCREASE"
	MoveDecrease PredictionMoveConfig = "DECREASE"
	MoveStagnate PredictionMoveConfig = "STAGNATE"
	MoveExpand   PredictionMoveConfig = "EXPAND"
	MoveDissolve PredictionMoveConfig = "DISSOLVE"
)

/*
PredictionConfig defines one falsifiable metric prediction in serializable form.
*/
type PredictionConfig struct {
	Metric     string               `json:"metric"`
	Support    PredictionMoveConfig `json:"support"`
	Contradict PredictionMoveConfig `json:"contradict"`
}

/*
FeatureConfig defines one advisor class, its observation keys, horizon, and predictions.
*/
type FeatureConfig struct {
	Class       string             `json:"class"`
	Within      uint64             `json:"within"`
	Keys        []string           `json:"keys"`
	Predictions []PredictionConfig `json:"predictions,omitempty"`
}

/*
AdvisorConfig defines the clock and features for one market advisor.
*/
type AdvisorConfig struct {
	Enabled  *bool           `json:"enabled,omitempty"`
	Clock    string          `json:"clock"`
	Features []FeatureConfig `json:"features"`
}

/*
AdvisorsConfig defines the complete registry of advisor configurations.
*/
type AdvisorsConfig struct {
	Advisors map[string]AdvisorConfig `json:"advisors"`
}

func parseMove(move PredictionMoveConfig) PredictedMove {
	switch move {
	case MoveIncrease:
		return INCREASE
	case MoveDecrease:
		return DECREASE
	case MoveStagnate:
		return STAGNATE
	case MoveExpand:
		return EXPAND
	case MoveDissolve:
		return DISSOLVE
	default:
		return NOMOVE
	}
}

func formatMove(move PredictedMove) PredictionMoveConfig {
	switch move {
	case INCREASE:
		return MoveIncrease
	case DECREASE:
		return MoveDecrease
	case STAGNATE:
		return MoveStagnate
	case EXPAND:
		return MoveExpand
	case DISSOLVE:
		return MoveDissolve
	default:
		return MoveNoMove
	}
}

/*
ToFeatures converts an AdvisorConfig into the internal Feature slices needed by NewSolver.
*/
func (advisorConfig AdvisorConfig) ToFeatures() []*Feature {
	features := make([]*Feature, 0, len(advisorConfig.Features))

	for _, featureConfig := range advisorConfig.Features {
		predictions := make([]*Prediction, 0, len(featureConfig.Predictions))

		for _, predConfig := range featureConfig.Predictions {
			predictions = append(predictions, NewMetricPrediction(
				predConfig.Metric,
				parseMove(predConfig.Support),
				parseMove(predConfig.Contradict),
			))
		}

		within := featureConfig.Within

		if within == 0 {
			within = 1
		}

		features = append(features, NewFeature(
			advisorConfig.Clock,
			featureConfig.Keys,
			&Class{
				Label:       featureConfig.Class,
				Within:      within,
				Predictions: predictions,
			},
		))
	}

	return features
}

/*
FromFeatures serializes an internal clock and Feature slice into an AdvisorConfig.
*/
func FromFeatures(clock string, features []*Feature) AdvisorConfig {
	featureConfigs := make([]FeatureConfig, 0, len(features))

	for _, feature := range features {
		var predConfigs []PredictionConfig
		var label string
		var within uint64

		if feature.Class != nil {
			label = feature.Class.Label
			within = feature.Class.Within

			for _, prediction := range feature.Class.Predictions {
				if prediction.Support != nil && prediction.Contradict != nil {
					predConfigs = append(predConfigs, PredictionConfig{
						Metric:     prediction.Support.Label,
						Support:    formatMove(prediction.Support.Move),
						Contradict: formatMove(prediction.Contradict.Move),
					})
				}
			}
		}

		featureConfigs = append(featureConfigs, FeatureConfig{
			Class:       label,
			Within:      within,
			Keys:        append([]string(nil), feature.Keys...),
			Predictions: predConfigs,
		})
	}

	return AdvisorConfig{
		Clock:    clock,
		Features: featureConfigs,
	}
}

/*
DefaultConfig builds the complete AdvisorsConfig from current compiled advisor definitions.
*/
func DefaultConfig() *AdvisorsConfig {
	config := &AdvisorsConfig{
		Advisors: make(map[string]AdvisorConfig),
	}

	config.Advisors[MomentumName] = FromFeatures(momentumClock, NewMomentum().Features)
	config.Advisors[AuctionName] = FromFeatures(auctionClock, NewAuction().Features)
	config.Advisors[ParticipationName] = FromFeatures(participationClock, NewParticipation().Features)
	config.Advisors[PullbackName] = FromFeatures(pullbackClock, NewPullback().Features)
	config.Advisors[ProfitRunName] = FromFeatures(profitRunClock, NewProfitRun().Features)
	config.Advisors[LiquidityName] = FromFeatures(liquidityClock, NewLiquidity().Features)
	config.Advisors[BasisName] = FromFeatures(basisClock, NewBasis().Features)

	return config
}

/*
FeaturesForAdvisor returns the Feature slice for the named advisor from the config,
falling back to the compiled defaults if the advisor or config is absent.
*/
func FeaturesForAdvisor(name string, config *AdvisorsConfig) []*Feature {
	if config != nil && config.Advisors != nil {
		if advisorCfg, found := config.Advisors[name]; found && len(advisorCfg.Features) > 0 {
			return advisorCfg.ToFeatures()
		}
	}

	switch name {
	case MomentumName:
		return NewMomentum().Features
	case AuctionName:
		return NewAuction().Features
	case ParticipationName:
		return NewParticipation().Features
	case PullbackName:
		return NewPullback().Features
	case ProfitRunName:
		return NewProfitRun().Features
	case LiquidityName:
		return NewLiquidity().Features
	case BasisName:
		return NewBasis().Features
	default:
		return nil
	}
}

/* Enabled reports whether a configured advisor occupies a council seat. */
func (config *AdvisorsConfig) Enabled(name string) bool {
	if config == nil || config.Advisors == nil {
		return true
	}

	advisorConfig, found := config.Advisors[name]

	if !found || advisorConfig.Enabled == nil {
		return true
	}

	return *advisorConfig.Enabled
}

/*
LoadConfig reads and parses an AdvisorsConfig JSON file from path.
*/
func LoadConfig(path string) (*AdvisorsConfig, error) {
	bytes, err := os.ReadFile(path)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			fmt.Sprintf("advisor: read config %s", path),
			err,
		))
	}

	var config AdvisorsConfig

	if err := json.Unmarshal(bytes, &config); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("advisor: decode config %s", path),
			err,
		))
	}

	return &config, nil
}
