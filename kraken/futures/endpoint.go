package futures

type EndpointType string

const (
	WebSocketURL EndpointType = "wss://futures.kraken.com/ws/v1"
	BookFeed                  = "book"
)

type PingMessage struct {
	Event string `json:"event"`
}

type SubscribeMessage struct {
	Event      string   `json:"event"`
	Feed       string   `json:"feed"`
	ProductIDs []string `json:"product_ids"`
}
