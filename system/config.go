package system

import (
	"sync/atomic"
)

var (
	SeqIdx atomic.Int64
	Tick   atomic.Int64
	Cfg    *Config
)

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
