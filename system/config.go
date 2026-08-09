package system

var Cfg *Config

func init() {
	Cfg = NewConfig()
}

type Config struct {
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