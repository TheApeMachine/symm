package logic

type RegimeType uint8

const (
	RegimeTypeNone RegimeType = iota
	RegimeTypeDead
	RegimeTypeChoppy
	RegimeTypeTrending
	RegimeTypeBullish
	RegimeTypeBearish
)

type Regime struct {
	Type RegimeType
}

func NewRegime(regimeType RegimeType) *Regime {
	return &Regime{Type: regimeType}
}
