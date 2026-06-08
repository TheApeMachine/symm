package logic

type EntityType uint8

const (
	EntityNone EntityType = iota
	EntitySymbol
	EntityTrade
	EntityTick
	EntityBook
)

type Entity struct {
	Type EntityType
}

func NewEntity(entityType EntityType) *Entity {
	return &Entity{Type: entityType}
}
