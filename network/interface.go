package network

import "encoding/json"

type Client interface {
	Read(...string) ([]byte, error)
	Write(message []byte) error
	Post(url string, body any) ([]byte, error)
	// Watch polls url and decodes each body into dest (e.g. *market.Balance).
	Watch(url string, body any, dest json.Unmarshaler)
}
