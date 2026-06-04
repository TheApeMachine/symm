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

/*
String returns the regime's lower-case dashboard name ("none" for RegimeNone).
regimeNames is defined alongside the other enum tables in encoding.go.
*/
func (regime Regime) String() string {
	return regimeNames[regime]
}
