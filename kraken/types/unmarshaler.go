package types

type Unmarshaler interface {
	Unmarshal(message *SocketMessage) error
}
