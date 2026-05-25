package engine

type PerspectiveType uint8

const (
	PerspectiveMicrostructure PerspectiveType = iota
	PerspectiveCrossAsset
	PerspectiveSentiment
)

type MarketRegime uint8

const (
	RegimeDead MarketRegime = iota
	RegimeTrending
	RegimeBullish
	RegimeBearish
)

type Perspective struct {
	Type         PerspectiveType
	Measurements []Measurement
	Regime       MarketRegime
}
