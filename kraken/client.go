package kraken

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/gofiber/fiber/v3"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/paper"
	"github.com/theapemachine/symm/kraken/private"
	"github.com/theapemachine/symm/kraken/public"
	"github.com/theapemachine/symm/kraken/replay"
)

type Client struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	rest   public.RestClient
	market public.WebSocketClient
	desk   public.WebSocketClient
	authMu sync.Mutex
	auth   public.WebSocketClient
}

func NewClient(ctx context.Context) (*Client, error) {
	ctx, cancel := context.WithCancel(ctx)

	rest, market, desk, err := newClientBackends(ctx)

	if err != nil {
		cancel()

		return nil, errnie.Error(err)
	}

	client := &Client{
		ctx:    ctx,
		cancel: cancel,
		rest:   rest,
		market: market,
		desk:   desk,
	}

	return client, errnie.Error(errnie.Require(map[string]any{
		"ctx":    client.ctx,
		"cancel": client.cancel,
		"rest":   client.rest,
		"market": client.market,
		"desk":   client.desk,
	}))
}

func newClientBackends(
	ctx context.Context,
) (public.RestClient, public.WebSocketClient, public.WebSocketClient, error) {
	switch viper.GetViper().Get("trading.model") {
	case "live":
		rest, err := private.NewRest(
			ctx,
			os.Getenv("SYMM_KRAKEN_API_KEY"),
			os.Getenv("SYMM_KRAKEN_API_SECRET"),
			public.EndpointAddOrder,
		)

		if err != nil {
			return nil, nil, nil, err
		}

		desk, err := private.NewWebSocket(
			ctx,
			os.Getenv("SYMM_KRAKEN_API_KEY"),
			os.Getenv("SYMM_KRAKEN_API_SECRET"),
		)

		if err != nil {
			return nil, nil, nil, err
		}

		return rest, desk, desk, nil
	case "replay":
		rest, err := replay.NewRest(ctx)

		if err != nil {
			return nil, nil, nil, err
		}

		desk, err := paper.NewWebSocket(ctx)

		if err != nil {
			return nil, nil, nil, err
		}

		return rest, nil, desk, nil
	default:
		rest, err := paper.NewRest(ctx)

		if err != nil {
			return nil, nil, nil, err
		}

		desk, err := paper.NewWebSocket(ctx)

		if err != nil {
			return nil, nil, nil, err
		}

		return rest, nil, desk, nil
	}
}

func (client *Client) socketFor(channel string) public.WebSocketClient {
	if channel == public.BalancesChannel {
		if auth := client.authSocket(); auth != nil {
			return auth
		}
	}

	if deskChannel(channel) {
		return client.desk
	}

	return client.market
}

func (client *Client) Stream(channel string) (<-chan *public.SocketMessage, error) {
	socket := client.socketFor(channel)

	if socket == nil {
		return nil, fmt.Errorf("channel %s not available", channel)
	}

	if err := client.ensureConnected(channel); err != nil {
		return nil, err
	}

	return socket.Stream(channel)
}

func (client *Client) Send(channel string, message any) error {
	socket := client.socketFor(channel)

	if socket == nil {
		return fmt.Errorf("channel %s not available", channel)
	}

	if err := client.ensureConnected(channel); err != nil {
		return err
	}

	return errnie.Error(socket.Send(channel, message))
}

func (client *Client) Request(message fiber.Map, model any) error {
	return errnie.Error(client.rest.Post(client.ctx, message, model))
}

func (client *Client) Close() error {
	client.cancel()

	return errnie.Error(client.ctx.Err())
}

func (client *Client) authSocket() public.WebSocketClient {
	apiKey := os.Getenv("SYMM_KRAKEN_API_KEY")
	apiSecret := os.Getenv("SYMM_KRAKEN_API_SECRET")

	if apiKey == "" || apiSecret == "" {
		return nil
	}

	client.authMu.Lock()
	defer client.authMu.Unlock()

	if client.auth != nil {
		return client.auth
	}

	auth, err := private.NewWebSocket(client.ctx, apiKey, apiSecret)

	if err != nil {
		errnie.Error(err)

		return nil
	}

	client.auth = auth

	return client.auth
}

func (client *Client) ensureConnected(channel string) error {
	socket := client.socketFor(channel)

	if socket == nil {
		return fmt.Errorf("channel %s not available", channel)
	}

	endpoint := public.WebSocketURL

	if deskChannel(channel) {
		endpoint = public.WebSocketAuthURL
	}

	return socket.Connect(endpoint, channel)
}

func deskChannel(channel string) bool {
	switch channel {
	case public.OrdersChannel, public.ExecutionsChannel, public.BalancesChannel:
		return true
	default:
		return false
	}
}
