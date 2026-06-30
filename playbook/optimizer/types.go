package optimizer

import "time"

type ProgressFunc func(format string, args ...any)

type Options struct {
	Iterations   int
	MaxDepth     int
	Exploration  float64
	CausalAlpha  float64
	InitialCash  float64
	FeeRate      float64
	MakerFeeRate float64
	MaxPositions int
	WriteTree    bool
	Progressf    ProgressFunc
}

type Report struct {
	Frames          int          `json:"frames"`
	Symbols         int          `json:"symbols"`
	Iterations      int          `json:"iterations"`
	MaxDepth        int          `json:"max_depth"`
	PlansEvaluated  int          `json:"plans_evaluated"`
	Baseline        PlanReport   `json:"baseline"`
	Best            PlanReport   `json:"best"`
	Recommendations []PlanReport `json:"recommendations"`
}

type PlanReport struct {
	Mutations   []string  `json:"mutations"`
	Reward      float64   `json:"reward"`
	Wallet      float64   `json:"wallet"`
	Cash        float64   `json:"cash"`
	Trades      int       `json:"trades"`
	Positions   int       `json:"positions"`
	MaxDrawdown float64   `json:"max_drawdown"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	EndedAt     time.Time `json:"ended_at,omitempty"`
}

type ReplayFrame struct {
	Time      time.Time          `json:"time"`
	Artifacts []ReplayArtifact   `json:"artifacts"`
	Prices    map[string]float64 `json:"prices,omitempty"`
}

type ReplayArtifact struct {
	Origin    string         `json:"origin,omitempty"`
	Role      string         `json:"role"`
	Scope     string         `json:"scope,omitempty"`
	Type      string         `json:"type,omitempty"`
	Timestamp int64          `json:"timestamp"`
	Payload   map[string]any `json:"payload"`
}

type ReplayResult struct {
	Reward      float64
	Wallet      float64
	Cash        float64
	Trades      int
	Positions   int
	MaxDrawdown float64
	StartedAt   time.Time
	EndedAt     time.Time
}

type Evaluator interface {
	Evaluate(treeYAML []byte) (ReplayResult, error)
}
