package replay

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/theapemachine/symm/config"
	symmreplay "github.com/theapemachine/symm/replay"
)

/*
Rest serves recorded Kraken REST payloads from a replay capture instead of dialing
the exchange.
*/
type Rest struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func NewRest(ctx context.Context) (*Rest, error) {
	ctx, cancel := context.WithCancel(ctx)

	return &Rest{ctx: ctx, cancel: cancel}, nil
}

func (rest *Rest) Get(
	ctx context.Context,
	request fiber.Map,
	model any,
	headers ...map[string]string,
) error {
	return rest.serve(request, model)
}

func (rest *Rest) Post(
	ctx context.Context,
	request fiber.Map,
	model any,
	headers ...map[string]string,
) error {
	return rest.serve(request, model)
}

func (rest *Rest) serve(request fiber.Map, model any) error {
	if model == nil {
		return nil
	}

	path := strings.TrimSpace(config.System.ReplayFile)

	if path == "" {
		return nil
	}

	hub, err := symmreplay.Open(path)

	if err != nil {
		return err
	}

	channel, ok := requestChannel(request)

	if !ok {
		return nil
	}

	body, ok := hub.RESTBody(channel)

	if !ok {
		return nil
	}

	return json.Unmarshal(body, model)
}

func requestChannel(request fiber.Map) (string, bool) {
	if request == nil {
		return "", false
	}

	if channel, ok := request["channel"].(string); ok && channel != "" {
		return channel, true
	}

	if method, ok := request["method"].(string); ok && method != "" {
		return method, true
	}

	return "", false
}

func (rest *Rest) Close() error {
	rest.cancel()

	return nil
}

func (rest *Rest) Error() error {
	return nil
}
