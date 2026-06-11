package logic

type EntityType string

const (
	EntityNone        EntityType = ""
	EntitySymbol      EntityType = "symbol"
	EntityTrade       EntityType = "trade"
	EntityTick        EntityType = "tick"
	EntityBook        EntityType = "book"
	EntityMeasurement EntityType = "measurement"
)

type Entity struct {
	Type EntityType `yaml:"type"`
}

func NewEntity(entityType EntityType) *Entity {
	return &Entity{Type: entityType}
}
