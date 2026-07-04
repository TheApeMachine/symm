package websocket

import (
	"context"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
)

/*
PrivateAccount is the account-side event source used by trading code.
Live mode is backed by Kraken's authenticated websocket. Paper mode is backed
by the Kraken CLI's local paper ledger and emits the same account channels.
*/
type PrivateAccount interface {
	Observe() chan map[string]any
	Submit(*datura.Artifact) error
	Sync() error
	Close()
}

func NewPrivateAccount(ctx context.Context) PrivateAccount {
	if viper.GetString("trading.model") == "live" {
		return NewAccount(ctx)
	}

	return NewPaperAccount(ctx)
}
