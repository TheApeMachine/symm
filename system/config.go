package system

import (
	"github.com/theapemachine/errnie"
)

var Cfg *Config

func init() {
	Cfg = NewConfig()
}

type Config struct {
	Runtime   *Runtime
	Resonance *Resonance
	Risk      *Risk
	Planner   *PlannerConfig
	PumpDump  *PumpDump
	CVD       *CVD
	Manifold  *ManifoldConfig
	WebSocket *WebSocket
	Market    *Market
	Learning  *Learning
	Storage   *Storage
}

func NewConfig() *Config {
	return &Config{
		Runtime:   NewRuntime(),
		Resonance: NewResonance(),
		Risk:      NewRisk(),
		Planner:   NewPlannerConfig(),
		PumpDump:  NewPumpDump(),
		CVD:       NewCVD(),
		Manifold:  NewManifoldConfig(),
		WebSocket: NewWebSocket(),
		Market:    NewMarket(),
		Learning:  NewLearning(),
		Storage:   NewStorage(),
	}
}

/* PlannerPolicy returns the small live policy value without allocating. */
func (config *Config) PlannerPolicy() (PlannerConfig, error) {
	if config == nil {
		return PlannerConfig{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"system: configuration required",
			nil,
		))
	}

	if config.Planner == nil {
		return PlannerConfig{}, errnie.Error(errnie.Err(
			errnie.Validation,
			"system: planner configuration required",
			nil,
		))
	}

	return *config.Planner, nil
}

/* CognitionSwitchConfidence returns cognition's configured state-switch policy. */
func (config *Config) CognitionSwitchConfidence() (float64, error) {
	policy, err := config.PlannerPolicy()

	if err != nil {
		return 0, err
	}

	return policy.CognitionSwitchConfidence, nil
}
