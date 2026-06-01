package market

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/public"
)

/*
Feed is a subscribed Kraken WebSocket channel. Callers range Stream and decode
rows from each SocketMessage.
*/
type Feed struct {
	Client *kraken.Client
	Stream <-chan *public.SocketMessage
}

/*
OpenFeed subscribes to one Kraken WebSocket channel and returns the client plus
its message stream for the caller to range.
*/
func OpenFeed(ctx context.Context, channel string, params any) Feed {
	client := errnie.Does(func() (*kraken.Client, error) {
		return kraken.NewClient(ctx)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	if err := client.Send(channel, public.Subscription{
		Method: public.MethodSubscribe,
		Params: params,
	}); err != nil {
		errnie.Error(err)
	}

	stream := errnie.Does(func() (<-chan *public.SocketMessage, error) {
		return client.Stream(channel)
	}).Or(func(err error) {
		errnie.Error(err)
	}).Value()

	return Feed{
		Client: client,
		Stream: stream,
	}
}
