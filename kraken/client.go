package kraken

import (
	"context"

	"github.com/gofiber/fiber/v3"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/paper"
	"github.com/theapemachine/symm/kraken/private"
	"github.com/theapemachine/symm/kraken/public"
)

type Client struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	rest   public.RestClient
	ws     public.WebSocketClient
}

func NewClient(ctx context.Context) (*Client, error) {
	ctx, cancel := context.WithCancel(ctx)

	rest := errnie.Does(func() (public.RestClient, error) {
		if viper.GetViper().Get("trading.model") == "paper" {
			return paper.NewRest(ctx)
		}

		return private.NewRest(
			ctx,
			config.System.KrakenAPIKey,
			config.System.KrakenAPISecret,
			public.EndpointAddOrder,
		)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	ws := errnie.Does(func() (public.WebSocketClient, error) {
		if viper.GetViper().Get("trading.model") == "paper" {
			return paper.NewWebSocket(ctx)
		}

		return public.NewWebSocket(ctx)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	client := &Client{
		ctx:    ctx,
		cancel: cancel,
		rest:   rest,
		ws:     ws,
	}

	return client, errnie.Error(errnie.Require(map[string]any{
		"ctx":    client.ctx,
		"cancel": client.cancel,
		"rest":   client.rest,
		"ws":     client.ws,
	}))
}

func (client *Client) Send(channel string, message any) error {
	return errnie.Error(client.ws.Send(channel, message))
}

func (client *Client) Request(message fiber.Map, model any) error {
	return errnie.Error(client.rest.Post(client.ctx, message, model))
}

func (client *Client) Close() error {
	client.cancel()
	return errnie.Error(client.ctx.Err())
}
