package logic

type EntityType string

const (
	EntityTypeNone   EntityType = ""
	EntityTypeTicker EntityType = "ticker"
	EntityTypeOrder  EntityType = "order"
	EntityTypeTrade  EntityType = "trade"
	EntityTypeBook   EntityType = "book"
	EntityTypeOHLC   EntityType = "ohlc"
)

type Entity struct {
	Type EntityType
}
