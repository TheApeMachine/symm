package network

import (
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/gofiber/fiber/v3/client"
	"github.com/theapemachine/errnie"
)

/*
PrivateSigner builds API-Key / API-Sign and a nonce-bearing body for one
private spot REST call. Wired at boot to the venue signer.
*/
type PrivateSigner func(requestURI string, body any) (key, signature string, signed []byte, err error)

var privateSigner atomic.Pointer[PrivateSigner]

func SetPrivateSigner(signer PrivateSigner) {
	privateSigner.Store(&signer)
}

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

func (rest *REST) Post(url string, body any) (*client.Response, error) {
	header := map[string]string{
		"Content-Type": "application/json",
	}

	requestURI, private, err := privateSpot(url)

	if err != nil {
		return nil, err
	}

	if private {
		signerPtr := privateSigner.Load()

		if signerPtr == nil || *signerPtr == nil {
			return nil, errnie.Err(
				errnie.Validation,
				"[network.rest] private signer is not configured",
				nil,
			)
		}

		key, signature, signed, err := (*signerPtr)(requestURI, body)

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
