package system

import (
	"errors"
	"sync"
)

var Cfg *Config

func init() {
	Cfg = NewConfig()
}

type Config struct {
	mu        sync.RWMutex
	Runtime   *Runtime
	Resonance *Resonance
	Risk      *Risk
	Planner   *PlannerConfig
	PumpDump  *PumpDump
	CVD       *CVD
	Manifold  *ManifoldConfig
	WebSocket *WebSocket
	Market    *Market
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
	}
}

/* PlannerPolicy returns the small live policy value without allocating. */
func (config *Config) PlannerPolicy() (PlannerConfig, error) {
	if config == nil {
		return PlannerConfig{}, errors.New("system: configuration required")
	}

	config.mu.RLock()
	defer config.mu.RUnlock()

	if config.Planner == nil {
		return PlannerConfig{}, errors.New("system: planner configuration required")
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
