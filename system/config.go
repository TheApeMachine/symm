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
	Resonance *Resonance
	Risk      *Risk
	Regulator *RegulatorConfig
	Planner   *PlannerConfig
	PumpDump  *PumpDump
	CVD       *CVD
	Manifold  *ManifoldConfig
	WebSocket *WebSocket
}

func NewConfig() *Config {
	return &Config{
		Resonance: NewResonance(),
		Risk:      NewRisk(),
		Regulator: NewRegulatorConfig(),
		Planner:   NewPlannerConfig(),
		PumpDump:  NewPumpDump(),
		CVD:       NewCVD(),
		Manifold:  NewManifoldConfig(),
		WebSocket: NewWebSocket(),
	}
}

/*
Snapshot returns an independent, internally consistent configuration reading.
*/
func (config *Config) Snapshot() *Config {
	if config == nil {
		return nil
	}

	config.mu.RLock()
	defer config.mu.RUnlock()

	snapshot := &Config{}

	if config.Resonance != nil {
		resonance := *config.Resonance
		snapshot.Resonance = &resonance
	}

	if config.Risk != nil {
		risk := *config.Risk
		snapshot.Risk = &risk
	}

	if config.Regulator != nil {
		regulator := *config.Regulator
		snapshot.Regulator = &regulator
	}

	if config.Planner != nil {
		planner := *config.Planner
		snapshot.Planner = &planner
	}

	return snapshot
}

/*
PlannerMinimumConfidence reads the current graph admission boundary without
allocating a configuration snapshot.
*/
func (config *Config) PlannerMinimumConfidence() (float64, error) {
	if config == nil {
		return 0, errors.New("system: configuration required")
	}

	config.mu.RLock()
	defer config.mu.RUnlock()

	if config.Planner == nil {
		return 0, errors.New("system: planner configuration required")
	}

	return config.Planner.MinimumConfidence, nil
}

/*
ApplyRegulation atomically publishes the regulator-owned dynamic settings.
*/
func (config *Config) ApplyRegulation(planner PlannerConfig) error {
	if config == nil || config.Planner == nil {
		return errors.New("system: planner configuration required for regulation")
	}

	config.mu.Lock()
	defer config.mu.Unlock()

	*config.Planner = planner

	return nil
}
