package system

var (
	Cfg *Config
)

func init() {
	Cfg = NewConfig()
}

type Config struct {
	Resonance *Resonance
	Risk      *Risk
	Selection *Selection
	WebSocket *WebSocket
	Market    *Market
	Learning  *Learning
	Storage   *Storage
	UI        *UI
	Workbench *Workbench
}

func NewConfig() *Config {
	return &Config{
		Resonance: NewResonance(),
		Risk:      NewRisk(),
		Selection: NewSelection(),
		WebSocket: NewWebSocket(),
		Market:    NewMarket(),
		Learning:  NewLearning(),
		Storage:   NewStorage(),
		UI:        NewUI(),
		Workbench: NewWorkbench(),
	}
}
