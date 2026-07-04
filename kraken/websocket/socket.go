package websocket

type Socket interface {
	Observe(string) chan []byte
}
