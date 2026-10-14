package network

type Client interface {
	Read(...string) ([]byte, error)
	Write(message []byte) error
}
