package system

var (
	Cfg *Config
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
	UI        *UI
	Workbench *Workbench
	Audit     *Audit
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
		UI:        NewUI(),
		Workbench: NewWorkbench(),
		Audit:     NewAudit(),
	}
}
