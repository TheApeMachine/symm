package logic

type PositionType uint8

const (
	PositionTypeNone PositionType = iota
	PositionTypeLong
	PositionTypeShort
)

type Position struct {
	Type PositionType `yaml:"type"`
}
