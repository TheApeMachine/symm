package kraken

import (
	"context"
	"fmt"
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

	rest := public.NewRest(ctx, public.PublicBaseURL)

	var (
		ws  public.WebSocketClient
		err error
	)

	switch viper.GetViper().GetString("trading.model") {
	case "record":
		ws = public.NewWebSocket(ctx, pool)
	case "replay":
		var file *os.File

		file, err = os.Open(viper.GetViper().GetString("trading.replay.file"))

		if err != nil {
			cancel()
			return nil, fmt.Errorf("kraken client: open replay file: %w", err)
		}

		ws, err = replay.NewWebSocket(ctx, pool, file)
	default:
		ws, err = paper.NewWebSocket(ctx, pool)
	}

	if err != nil {
		cancel()
		return nil, fmt.Errorf("kraken client: websocket: %w", err)
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
	if c.ws == nil {
		return fmt.Errorf("kraken client: websocket is nil")
	}

	return c.ws.Connect(endpoint, channel)
}

func (c *Client) Tick() error {
	if c.ws == nil {
		return fmt.Errorf("kraken client: websocket is nil")
	}

	return c.ws.Tick()
}

func (c *Client) Close() error {
	c.cancel()

	if c.ws == nil {
		return fmt.Errorf("kraken client: websocket is nil")
	}

	return c.ws.Close()
}
