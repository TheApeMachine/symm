package network

import (
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v3/client"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
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
	if _, private, err := privateSpot(rawURL); err != nil || private {
		if err != nil {
			return nil, err
		}

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

func (rest *REST) Post(rawURL string, body any) (*client.Response, error) {
	header := map[string]string{
		"Content-Type": "application/json",
	}

	requestURI, private, err := privateSpot(rawURL)

	if err != nil {
		return nil, err
	}

	if private {
		key, signature, signed, err := kraken.SignPrivate(requestURI, body)

		if err != nil {
			return nil, err
		}

		header["API-Key"] = key
		header["API-Sign"] = signature
		body = signed
	}

	return rest.conn.Post(rawURL, client.Config{
		Header: header,
		Body:   body,
	})
}

/*
privateSpot reports whether rawURL is a Kraken spot private endpoint.
The returned request URI is the path and query Kraken expects in the signature.
*/
func privateSpot(rawURL string) (string, bool, error) {
	parsed, err := url.Parse(rawURL)

	if err != nil {
		return "", false, errnie.Err(errnie.Validation, "[network.rest] url is invalid", err)
	}

	if !strings.Contains(parsed.Path, "/private/") {
		return "", false, nil
	}

	if parsed.Host != "api.kraken.com" {
		return "", false, errnie.Err(
			errnie.Validation,
			"[network.rest] no signer for private url on "+parsed.Host,
			nil,
		)
	}

	return parsed.RequestURI(), true, nil
}
