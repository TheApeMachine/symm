package types

import "go.yaml.in/yaml/v3"

type Regime uint8

const (
	RegimeNone Regime = iota
	RegimeDead
	RegimeChoppy
	RegimeTrending
	RegimeBullish
	RegimeBearish
)

var regimeNames = map[Regime]string{
	RegimeNone:     "none",
	RegimeDead:     "dead",
	RegimeChoppy:   "choppy",
	RegimeTrending: "trending",
	RegimeBullish:  "bullish",
	RegimeBearish:  "bearish",
}

/*
String returns the regime's lower-case dashboard name ("none" for RegimeNone).
*/
func (regime Regime) String() string {
	return regimeNames[regime]
}

func (regime Regime) MarshalYAML() (any, error) {
	return MarshalEnum(regime, regimeNames)
}

func (regime Regime) MarshalJSON() ([]byte, error) {
	return MarshalEnumJSON(regime, regimeNames)
}

func (regime *Regime) UnmarshalYAML(value *yaml.Node) error {
	return UnmarshalEnum(value, regime, regimeNames)
}

func (regime *Regime) UnmarshalJSON(data []byte) error {
	return UnmarshalEnumJSON(data, regime, regimeNames)
}
