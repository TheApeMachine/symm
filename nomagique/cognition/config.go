package cognition

/*
Config declares explicit mathematical bounds for the cognitive engine.
*/
type Config struct {
	// MemoryScale M sets the exponential decay factor: λ = 1 - 1/M.
	// If M <= 1, decay is disabled (cumulative infinite memory).
	MemoryScale float64

	// DirichletAlpha α is the pseudo-count prior for unseen transitions.
	// α = 1.0 corresponds to Laplace smoothing; α = 0.5 to Jeffreys prior.
	DirichletAlpha float64

	// MaxBackoffOrder bounds the n-gram suffix walk (typically 4-6).
	MaxBackoffOrder int

	// BeamWidth and MaxHops govern the lookahead search.
	BeamWidth int
	MaxHops   int

	// SurprisalBreakBits is the information threshold (-log2 P) where a
	// transition is considered an unexpected regime break.
	SurprisalBreakBits float64
}

func DefaultConfig() Config {
	return Config{
		MemoryScale:        2000.0, // λ = 1 - 1/2000 = 0.9995
		DirichletAlpha:     0.5,    // Jeffreys uninformative prior
		MaxBackoffOrder:    4,
		BeamWidth:          3,
		MaxHops:            2,
		SurprisalBreakBits: 3.5, // P < 8.8%
	}
}

/*
DecayFactor computes λ = 1 - 1/M.
*/
func (c Config) DecayFactor() float64 {
	if c.MemoryScale <= 1.0 {
		return 1.0
	}
	return 1.0 - (1.0 / c.MemoryScale)
}
