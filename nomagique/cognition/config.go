package cognition

/*
Config declares the mathematical bounds of the cognitive engine as a plain
payload. The engine normalizes unset bounds to its declared defaults when it
is constructed; normalized values are computed inside the primitive.
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

/*
defaultBounds is the declared default configuration. An unset bound is not a
chosen zero, and reading it as one is how a model comes to claim it decided
something it was never configured for; the constructor folds these in.
*/
func defaultBounds() Config {
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
normalised fills unset bounds from the declared defaults.
*/
func (c Config) normalised() Config {
	fallback := defaultBounds()

	if c.MemoryScale == 0 {
		c.MemoryScale = fallback.MemoryScale
	}

	if c.DirichletAlpha <= 0 {
		c.DirichletAlpha = fallback.DirichletAlpha
	}

	if c.MaxBackoffOrder <= 0 {
		c.MaxBackoffOrder = fallback.MaxBackoffOrder
	}

	if c.BeamWidth <= 0 {
		c.BeamWidth = fallback.BeamWidth
	}

	if c.MaxHops <= 0 {
		c.MaxHops = fallback.MaxHops
	}

	if c.SurprisalBreakBits <= 0 {
		c.SurprisalBreakBits = fallback.SurprisalBreakBits
	}

	return c
}

/*
decayFactor computes λ = 1 - 1/M for the memory scale M.
*/
func (c Config) decayFactor() float64 {
	if c.MemoryScale <= 1.0 {
		return 1.0
	}

	return 1.0 - (1.0 / c.MemoryScale)
}
