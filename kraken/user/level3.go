package user

import (
	"context"

	"github.com/theapemachine/symm/kraken/public"
)

/*
Level3Params is the Kraken WebSocket v2 subscribe payload for the level3 channel.
*/
type Level3Params struct {
	Channel  string   `json:"channel"`
	Symbol   []string `json:"symbol"`
	Depth    int      `json:"depth,omitempty"`
	Snapshot bool     `json:"snapshot"`
	Token    string   `json:"token"`
}

/*
Level3SubscribeFrame subscribes to per-order L3 book updates on ws-l3.
*/
type Level3SubscribeFrame struct {
	Method string       `json:"method"`
	Params Level3Params `json:"params"`
}

/*
NewLevel3SubscribeFrame builds an authenticated level3 subscribe request.
*/
func NewLevel3SubscribeFrame(
	symbols []string,
	depth int,
	token string,
) Level3SubscribeFrame {
	return Level3SubscribeFrame{
		Method: "subscribe",
		Params: Level3Params{
			Channel:  public.Level3Channel,
			Symbol:   symbols,
			Depth:    depth,
			Snapshot: true,
			Token:    token,
		},
	}
}

/*
SubscribeLevel3 sends one level3 subscribe frame on the private websocket bus.
*/
func SubscribeLevel3(
	ctx context.Context,
	tokenSource TokenSource,
	privateBus func(frame Level3SubscribeFrame),
	symbols []string,
	depth int,
) error {
	if tokenSource == nil || privateBus == nil || len(symbols) == 0 {
		return nil
	}

	token, err := tokenSource.Token(ctx)

	if err != nil {
		return err
	}

	privateBus(NewLevel3SubscribeFrame(symbols, depth, token))

	return nil
}
