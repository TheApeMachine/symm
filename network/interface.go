package network

import "encoding/json"

type Client interface {
	Read(...string) ([]byte, error)
	Write(message []byte) error
	Post(url string, body any) ([]byte, error)
	// Watch polls url, decodes into dest, then runs after (may be nil).
	Watch(url string, body any, dest json.Unmarshaler, after func())
}
