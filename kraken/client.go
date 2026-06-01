package kraken

import (
	"context"
	"os"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/kraken/paper"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/replay"
)

type Client struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	rest   public.RestClient
	ws     public.WebSocketClient
}

func NewClient(ctx context.Context, pool *qpool.Q) (*Client, error) {
	ctx, cancel := context.WithCancel(ctx)

	var rest public.RestClient
	var ws public.WebSocketClient

	rest = errnie.Does(func() (public.RestClient, error) {
		return public.NewRest(ctx, public.PublicBaseURL), nil
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	switch viper.GetViper().GetString("trading.model") {
	case "record":
		ws = public.NewWebSocket(ctx, pool)
	case "replay":
		file := errnie.Does(func() (*os.File, error) {
			return os.Open(viper.GetViper().GetString("trading.replay.file"))
		}).Or(func(err error) {
			errnie.Error(err)
		}).Value()

		ws = errnie.Does(func() (public.WebSocketClient, error) {
			return replay.NewWebSocket(ctx, pool, file)
		}).Or(func(err error) {
			errnie.Error(err)
		}).Value()
	default:
		ws = errnie.Does(func() (public.WebSocketClient, error) {
			return paper.NewWebSocket(ctx, pool)
		}).Or(func(err error) {
			errnie.Error(err)
		}).Value()
	}

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

func (c *Client) Connect(endpoint public.EndpointType, channel string) error {
	return c.ws.Connect(endpoint, channel)
}

func (c *Client) Tick() error {
	return c.ws.Tick()
}

func (c *Client) Close() error {
	c.cancel()
	return c.ws.Close()
}
