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
	Planner *PlannerConfig
}

func NewConfig() *Config {
	return &Config{
		Resonance: NewResonance(),
		Manifold:  NewManifold(),
		Risk:      NewRisk(),
		Planner:   NewPlannerConfig(),
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
	resonance Resonance,
	planner PlannerConfig,
) error {
	if config == nil || config.Resonance == nil || config.Planner == nil {
		return errors.New("system: resonance and planner configuration required for regulation")
	}

	config.mu.Lock()
	defer config.mu.Unlock()

	*config.Resonance = resonance
	*config.Planner = planner

	return nil
}