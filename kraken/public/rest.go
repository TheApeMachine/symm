package public

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/bytedance/sonic"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/client"
	"github.com/theapemachine/errnie"
)

type RestClient interface {
	Get(ctx context.Context, request fiber.Map, model any, headers ...map[string]string) error
	Post(ctx context.Context, request fiber.Map, model any, headers ...map[string]string) error
	Error() error
	Close() error
}

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

func NewRest(
	ctx context.Context,
	endpoint EndpointType,
) *Rest {
	ctx, cancel := context.WithCancel(ctx)

	return &Rest{
		ctx:      ctx,
		cancel:   cancel,
		client:   client.New(),
		endpoint: endpoint,
	}
}

func (rest *Rest) Get(
	ctx context.Context,
	request fiber.Map,
	model any,
	headers ...map[string]string,
) error {
	errnie.Debug("kraken.public.rest.Get", request, model)
	params := url.Values{}

	for key, value := range request {
		params.Add(key, fmt.Sprintf("%v", value))
	}

	header := map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}

	for _, h := range headers {
		for key, value := range h {
			header[key] = value
		}
	}

	response := errnie.Does(func() (*client.Response, error) {
		return rest.client.Get(strings.Join([]string{
			string(rest.endpoint), params.Encode(),
		}, "?"), client.Config{
			Ctx:     rest.ctx,
			Timeout: 3 * time.Second,
			Header:  header,
		})
	}).Or(func(err error) {
		rest.err = errnie.Error(err)
	})

	if rest.err != nil {
		return rest.err
	}

	resp := response.Value()

	if resp == nil {
		rest.err = fmt.Errorf("kraken public rest: empty response")

		return rest.err
	}

	defer resp.Close()

	envelope := Response{Result: model}

	if err := sonic.Unmarshal(resp.Body(), &envelope); err != nil {
		rest.err = errnie.Error(err)

		return rest.err
	}

	if len(envelope.Error) > 0 && envelope.Error[0] != "" {
		rest.err = fmt.Errorf("%s", strings.Join(envelope.Error, ", "))

		return rest.err
	}

	return nil
}

func (rest *Rest) Post(
	ctx context.Context,
	request fiber.Map,
	model any,
	headers ...map[string]string,
) error {
	errnie.Debug("kraken.public.rest.Post", request, model)

	header := map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	}

	for _, h := range headers {
		for key, value := range h {
			header[key] = value
		}
	}

	response := errnie.Does(func() (*client.Response, error) {
		return rest.client.Post(string(rest.endpoint), client.Config{
			Ctx:     rest.ctx,
			Timeout: 3 * time.Second,
			Header:  header,
			Body:    request,
		})
	}).Or(func(err error) {
		rest.err = errnie.Error(err)
	})

	if rest.err != nil {
		return rest.err
	}

	resp := response.Value()

	if resp == nil {
		rest.err = fmt.Errorf("kraken public rest: empty response")

		return rest.err
	}

	defer resp.Close()

	envelope := Response{Result: model}

	if err := sonic.Unmarshal(resp.Body(), &envelope); err != nil {
		rest.err = errnie.Error(err)

		return rest.err
	}

	if len(envelope.Error) > 0 && envelope.Error[0] != "" {
		rest.err = fmt.Errorf("%s", strings.Join(envelope.Error, ", "))

		return rest.err
	}

	return nil
}

func (rest *Rest) Error() error {
	return errnie.Error(rest.err)
}

func (rest *Rest) Close() error {
	rest.cancel()
	return errnie.Error(rest.ctx.Err())
}
