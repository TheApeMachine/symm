package public

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/client"
	"github.com/theapemachine/errnie"
)

/*
Rest is the REST client for the Kraken public API.
*/
type Rest struct {
	ctx      context.Context
	cancel   context.CancelFunc
	err      error
	client   *client.Client
	endpoint EndpointType
}

func NewRest(ctx context.Context, endpoint EndpointType) *Rest {
	ctx, cancel := context.WithCancel(ctx)

	return &Rest{
		ctx:      ctx,
		cancel:   cancel,
		client:   client.New(),
		endpoint: endpoint,
	}
}

func (rest *Rest) Get(
	ctx context.Context, request fiber.Map, model any,
) error {
	params := url.Values{}

	for key, value := range request {
		params.Add(key, fmt.Sprintf("%v", value))
	}

	response := errnie.Does(func() (*client.Response, error) {
		return rest.client.Get(strings.Join([]string{
			string(rest.endpoint), params.Encode(),
		}, "?"), client.Config{
			Ctx:     rest.ctx,
			Timeout: 3 * time.Second,
			Header: map[string]string{
				"Content-Type": "application/json",
				"Accept":       "application/json",
			},
		})
	}).Or(func(err error) {
		rest.err = errnie.Error(err)
	})

	defer response.Value().Close()

	return errnie.Error(json.Unmarshal(
		response.Value().Body(), &Response{Result: model},
	))
}

func (rest *Rest) Error() error {
	return errnie.Error(rest.err)
}

func (rest *Rest) Close() error {
	rest.cancel()
	return errnie.Error(rest.ctx.Err())
}
