package network

import (
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3/client"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/venue/kraken"
)

const restTimeout = 10 * time.Second

type REST struct {
	conn *client.Client
}

func NewREST() *REST {
	conn := client.New()
	conn.SetTimeout(restTimeout)

	return &REST{conn: conn}
}

/*
restTransient is a dead keep-alive / early close from the peer — one retry on a
fresh dial usually clears it. Not a venue reject and must not burn the counter.
*/
func restTransient(err error) bool {
	if err == nil {
		return false
	}

	msg := strings.ToLower(err.Error())

	return strings.Contains(msg, "server closed connection before returning") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "eof")
}

func (rest *REST) Get(rawURL string, params map[string]string) (*client.Response, error) {
	if strings.Contains(rawURL, "/private/") {
		return nil, errnie.Err(
			errnie.Validation,
			"[network.rest] private endpoints are POST",
			nil,
		)
	}

	response, err := rest.conn.Get(rawURL, client.Config{
		Header: map[string]string{
			"Content-Type": "application/json",
		},
		Param: params,
	})

	return response, err
}

func (rest *REST) Post(url string, body any) (*client.Response, error) {
	header := map[string]string{
		"Content-Type": "application/json",
	}

	private := strings.Contains(url, "/private/")

	if private {
		cost := kraken.Private.Cost(url)

		if err := kraken.Private.Hold(context.Background(), cost); err != nil {
			return nil, err
		}
		defer kraken.Private.Release()

		key, signature, signed, err := kraken.SignPrivateURL(url, body)

		if err != nil {
			return nil, err
		}

		header["API-Key"] = key
		header["API-Sign"] = signature
		body = signed
	}

	cfg := client.Config{
		Header: header,
		Body:   body,
	}

	response, err := rest.conn.Post(url, cfg)

	// fasthttp reuses sockets; Kraken often closes idle ones between Balance
	// polls. One immediate retry avoids ERROR spam and a missed wallet read.
	if err != nil && restTransient(err) {
		response, err = rest.conn.Post(url, cfg)
	}

	if private {
		if err != nil {
			kraken.Private.ObserveError(err)
		} else if response != nil {
			kraken.Private.Commit(kraken.Private.Cost(url))
			kraken.Private.Observe(kraken.VenueErrors(response.Body())...)
		}
	}

	return response, err
}
