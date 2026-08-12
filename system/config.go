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
	mu sync.RWMutex
	*Resonance
	*Manifold
	*Risk
	Regulator *RegulatorConfig
	Planner   *PlannerConfig
	*PumpDump
	*CVD
}

func NewConfig() *Config {
	return &Config{
		Resonance: NewResonance(),
		Manifold:  NewManifold(),
		Risk:      NewRisk(),
		Regulator: NewRegulatorConfig(),
		Planner:   NewPlannerConfig(),
		PumpDump:  NewPumpDump(),
		CVD:       NewCVD(),
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

	if config.Manifold != nil {
		manifold := *config.Manifold
		snapshot.Manifold = &manifold
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
ApplyRegulation atomically publishes the regulator-owned dynamic settings.
*/
func (config *Config) ApplyRegulation(
	manifold Manifold,
	planner PlannerConfig,
) error {
	if config == nil || config.Manifold == nil || config.Planner == nil {
		return errors.New("system: manifold and planner configuration required for regulation")
	}

	config.mu.Lock()
	defer config.mu.Unlock()

	*config.Manifold = manifold
	*config.Planner = planner

	return nil
}
