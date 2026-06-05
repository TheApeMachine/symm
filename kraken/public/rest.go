package public

import (
	"context"
	"encoding/json"
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

type restEnvelope struct {
	Error  []string        `json:"error"`
	Result json.RawMessage `json:"result"`
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
		Ctx:     ctx,
		Timeout: 3 * time.Second,
		Header:  header,
	}); rest.err != nil {
		return errnie.Error(rest.err)
	}

	defer response.Close()

	rest.err = decodeKrakenEnvelope(response.Body(), model)
	return errnie.Error(rest.err)
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
		Ctx:     ctx,
		Timeout: 3 * time.Second,
		Header:  header,
		Body:    request}); rest.err != nil {
		return errnie.Error(rest.err)
	}

	defer response.Close()

	rest.err = decodeKrakenEnvelope(response.Body(), model)
	return errnie.Error(rest.err)
}

func decodeKrakenEnvelope(body []byte, model any) error {
	envelope := restEnvelope{}

	if err := sonic.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("kraken rest envelope: %w", err)
	}

	if len(envelope.Error) > 0 {
		return fmt.Errorf("kraken rest: %s", strings.Join(envelope.Error, "; "))
	}

	if model == nil {
		return nil
	}

	if len(envelope.Result) == 0 || string(envelope.Result) == "null" {
		return fmt.Errorf("kraken rest: missing result")
	}

	if err := sonic.Unmarshal(envelope.Result, model); err != nil {
		return fmt.Errorf("kraken rest result: %w", err)
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
