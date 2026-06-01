package user

import (
	"context"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/public"
)

const balanceSnapshot = "snapshot"

/*
BalanceTokenSource supplies short-lived authenticated WebSocket tokens.
*/
type BalanceTokenSource interface {
	Token(context.Context) (string, error)
}

/*
BalanceParams is the Kraken WebSocket v2 subscribe payload for the balances channel.
*/
type BalanceParams struct {
	Channel  string `json:"channel"`
	Snapshot bool   `json:"snapshot"`
	Token    string `json:"token"`
	Rebased  bool   `json:"rebased,omitempty"`
	Users    string `json:"users,omitempty"`
}

/*
BalanceWallet is one wallet holding for an asset in a balances snapshot.
*/
type BalanceWallet struct {
	Balance float64 `json:"balance"`
	Type    string  `json:"type"`
	ID      string  `json:"id"`
}

/*
Balance is one asset row from the balances channel snapshot or update.

Snapshots carry wallets and total holdings per asset; updates carry ledger
fields (amount, fee, ledger_id, ref_id, category, wallet_type, wallet_id).
EnvelopeType records snapshot vs update from the channel message.
*/
type Balance struct {
	Asset        string          `json:"asset"`
	AssetClass   string          `json:"asset_class"`
	Holdings     float64         `json:"balance"`
	Wallets      []BalanceWallet `json:"wallets,omitempty"`
	Amount       float64         `json:"amount,omitempty"`
	Fee          float64         `json:"fee,omitempty"`
	LedgerID     string          `json:"ledger_id,omitempty"`
	RefID        string          `json:"ref_id,omitempty"`
	Timestamp    string          `json:"timestamp,omitempty"`
	LedgerType   string          `json:"type,omitempty"`
	Subtype      string          `json:"subtype,omitempty"`
	Category     string          `json:"category,omitempty"`
	WalletType   string          `json:"wallet_type,omitempty"`
	WalletID     string          `json:"wallet_id,omitempty"`
	User         string          `json:"user,omitempty"`
	EnvelopeType string          `json:"-"`
}

func (balance *Balance) SetEnvelopeType(kind string) {
	balance.EnvelopeType = kind
}

func (balance *Balance) IsSnapshot() bool {
	return balance.EnvelopeType == balanceSnapshot
}

var (
	balanceTokenSourceMu sync.RWMutex
	balanceTokenSource   BalanceTokenSource
)

func SetBalanceTokenSource(source BalanceTokenSource) {
	balanceTokenSourceMu.Lock()
	defer balanceTokenSourceMu.Unlock()

	balanceTokenSource = source
}

func BalanceAvailable() bool {
	balanceTokenSourceMu.RLock()
	defer balanceTokenSourceMu.RUnlock()

	return balanceTokenSource != nil
}

func NewBalanceSubscription(ctx context.Context) <-chan *Balance {
	balanceTokenSourceMu.RLock()
	source := balanceTokenSource
	balanceTokenSourceMu.RUnlock()

	if source == nil {
		return nil
	}

	token, err := source.Token(ctx)

	if err != nil {
		errnie.Error(err)

		return nil
	}

	out := make(chan *Balance, 128)

	go func() {
		defer close(out)

		client, err := kraken.NewClient(ctx)

		if err != nil {
			errnie.Error(err)

			return
		}

		if err := client.Send(public.BalancesChannel, public.Subscription{
			Method: public.MethodSubscribe,
			Params: BalanceParams{
				Channel:  public.BalancesChannel,
				Snapshot: true,
				Token:    token,
			},
		}); err != nil {
			errnie.Error(err)

			return
		}

		stream, err := client.Stream(public.BalancesChannel)

		if err != nil {
			errnie.Error(err)

			return
		}

		for msg := range stream {
			if msg == nil {
				continue
			}

			var rows []Balance

			if err := sonic.Unmarshal(msg.Data, &rows); err != nil {
				errnie.Error(err)
				continue
			}

			for index := range rows {
				rows[index].SetEnvelopeType(msg.Type)
				out <- &rows[index]
			}
		}
	}()

	return out
}
