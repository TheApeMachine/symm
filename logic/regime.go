package logic

type RegimeType string

const (
	RegimeTypeNone     RegimeType = ""
	RegimeTypeDead     RegimeType = "dead"
	RegimeTypeChoppy   RegimeType = "choppy"
	RegimeTypeTrending RegimeType = "trending"
	RegimeTypeBullish  RegimeType = "bullish"
	RegimeTypeBearish  RegimeType = "bearish"
)

type Regime struct {
	Type RegimeType `yaml:"type" json:"type"`
}

func NewRegime(regimeType RegimeType) *Regime {
	return &Regime{Type: regimeType}
}
