package network

import "encoding/json"

type Client interface {
	Read(...string) ([]byte, error)
	Write(message []byte) error
	Post(url string, body any) ([]byte, error)
	Watch(url string, body any, callback func(json.Unmarshaler))
}
