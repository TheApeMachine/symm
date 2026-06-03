package public

import (
	"context"
	"fmt"
	"maps"
	"net/url"
	"strings"
	"sync"
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

var responsePool = sync.Pool{
	New: func() any {
		return &client.Response{}
	},
}

var envelopePool = sync.Pool{
	New: func() any {
		return make(map[string]any)
	},
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
		maps.Copy(header, h)
	}

	response := responsePool.Get().(*client.Response)
	defer responsePool.Put(response)

	if response, rest.err = rest.client.Get(strings.Join([]string{
		string(rest.endpoint), params.Encode(),
	}, "?"), client.Config{
		Ctx:     rest.ctx,
		Timeout: 3 * time.Second,
		Header:  header,
	}); rest.err != nil {
		return errnie.Error(rest.err)
	}

	defer response.Close()

	envelope := envelopePool.Get().(map[string]any)
	defer envelopePool.Put(envelope)

	if rest.err = sonic.Unmarshal(response.Body(), envelope); rest.err != nil {
		return errnie.Error(rest.err)
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
		maps.Copy(header, h)
	}

	response := responsePool.Get().(*client.Response)
	defer responsePool.Put(response)

	if response, rest.err = rest.client.Post(string(rest.endpoint), client.Config{
		Ctx:     rest.ctx,
		Timeout: 3 * time.Second,
		Header:  header,
		Body:    request}); rest.err != nil {
		return errnie.Error(rest.err)
	}

	defer response.Close()

	envelope := envelopePool.Get().(map[string]any)
	defer envelopePool.Put(envelope)

	if rest.err = sonic.Unmarshal(response.Body(), envelope); rest.err != nil {
		return errnie.Error(rest.err)
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
