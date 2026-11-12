package network

import (
	"strings"

	"github.com/gofiber/fiber/v3/client"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/venue/kraken"
)

type REST struct {
	conn *client.Client
}

func NewREST() *REST {
	return &REST{
		conn: client.New(),
	}
}

func (rest *REST) Get(rawURL string, params map[string]string) (*client.Response, error) {
	if strings.Contains(rawURL, "/private/") {
		return nil, errnie.Err(
			errnie.Validation,
			"[network.rest] private endpoints are POST",
			nil,
		)
	}

	return rest.conn.Get(rawURL, client.Config{
		Header: map[string]string{
			"Content-Type": "application/json",
		},
		Param: params,
	})
}

func (rest *REST) Post(url string, body any) (*client.Response, error) {
	header := map[string]string{
		"Content-Type": "application/json",
	}

	if strings.Contains(url, "/private/") {
		kraken.PrivateRESTMu.Lock()
		defer kraken.PrivateRESTMu.Unlock()

		key, signature, signed, err := kraken.SignPrivateURL(url, body)

		if err != nil {
			return nil, err
		}

		header["API-Key"] = key
		header["API-Sign"] = signature
		body = signed
	}

	return rest.conn.Post(url, client.Config{
		Header: header,
		Body:   body,
	})
}
