package logic

type EntityType uint8

const (
	EntityNone EntityType = iota
	EntitySymbol
	EntityTrade
	EntityTick
	EntityBook
	EntityMeasurement
)

type Entity struct {
	Type EntityType `yaml:"type"`
}

func NewEntity(entityType EntityType) *Entity {
	return &Entity{Type: entityType}
}
