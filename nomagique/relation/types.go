package relation

import "time"

type PlanScope struct {
	Symbol string
	Epoch  uint64
}

type Candidate struct {
	Source           string
	Target           string
	Controls         []string
	ControlLags      []float64
	MinLag           float64
	MaxLag           float64
	ControlsComplete bool
}

type Request struct {
	Source      string
	Target      string
	Controls    []string
	ControlLags []time.Duration
	MinLag      time.Duration
	MaxLag      time.Duration
}

type Estimate struct {
	Status           int
	Lag              float64
	EstimatorVersion string
	Metrics          map[string]float64
}
