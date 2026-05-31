package trader

import (
	"context"
	"fmt"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/order"
	"github.com/theapemachine/symm/wallet"
)

/*
liveSession routes orders through Kraken WebSocket v2 and reconciles fills.
*/
type liveSession struct {
	orderSession
	client *order.Client
	router *broker.Router
}

/*
NewLiveSession starts the authenticated trading socket when credentials exist.
*/
func NewLiveSession(ctx context.Context, apiKey, apiSecret string) (*liveSession, error) {
	client, err := order.NewClient(ctx, apiKey, apiSecret)

	if err != nil {
		return nil, err
	}

	session := &liveSession{client: client}
	session.router = broker.NewRouter(func(value any) error {
		switch frame := value.(type) {
		case order.Request:
			return client.Publish(frame)
		case order.CancelRequest:
			return client.PublishCancel(frame)
		default:
			return fmt.Errorf("live order router: unsupported frame type %T", value)
		}
	})

	if err := client.Start(); err != nil {
		_ = client.Close()

		return nil, err
	}

	return session, nil
}

func (session *liveSession) Router() *broker.Router {
	return session.router
}

func (session *liveSession) Fills() <-chan order.Fill {
	return session.client.Fills()
}

func (session *liveSession) Acks() <-chan order.Ack {
	return session.client.Acks()
}

func (session *liveSession) Close() error {
	return session.client.Close()
}

func liveEnabled(tradingWallet *wallet.Wallet) bool {
	if tradingWallet == nil || tradingWallet.Type != wallet.CryptoWallet {
		return false
	}

	if config.System.KrakenAPIKey == "" || config.System.KrakenAPISecret == "" {
		return false
	}

	return config.System.LiveTradingEnabled
}
