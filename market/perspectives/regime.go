package perspectives

type Regime uint8

const (
	RegimeNone Regime = iota
	RegimeDead
	RegimeChoppy
	RegimeTrending
	RegimeBullish
	RegimeBearish
)
