package logic

type PositionType string

const (
	PositionTypeNone  PositionType = ""
	PositionTypeLong  PositionType = "long"
	PositionTypeShort PositionType = "short"
)

type Position struct {
	Type PositionType `yaml:"type"`
}
