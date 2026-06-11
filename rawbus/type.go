package rawbus

/*
Type names frames on the raw broadcast bus.
Use these constants instead of string literals for routing and switches.
*/
type Type string

const (
	TypeActions      Type = "actions"
	TypeOrder        Type = "order"
	TypeBalances     Type = "balances"
	TypeOrders       Type = "orders"
	TypeExecutions   Type = "executions"
	TypeSymbols      Type = "symbols"
	TypeInstrument   Type = "instrument"
	TypeTicker       Type = "ticker"
	TypeTrade        Type = "trade"
	TypeBook         Type = "book"
	TypeOHLC         Type = "ohlc"
	TypeLevel3       Type = "level3"
	TypeFeedback     Type = "feedback"
	TypeExecution    Type = "execution"
	TypeMeasurements Type = "measurements"
	TypeReconnect    Type = "reconnect"
)

func (messageType Type) String() string {
	return string(messageType)
}

func TypeFrom(messageType string) Type {
	return Type(messageType)
}
