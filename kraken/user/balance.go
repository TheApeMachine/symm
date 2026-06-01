package user

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/kraken/public"
)

const balanceSnapshot = "snapshot"

/*
TokenSource supplies short-lived authenticated WebSocket tokens for balances.
*/
type TokenSource interface {
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

/*
NewBalanceSubscription subscribes to the balances channel using source for auth.
*/
func NewBalanceSubscription(ctx context.Context, source TokenSource) market.Feed {
	if source == nil {
		return market.Feed{}
	}

	token, err := source.Token(ctx)

	if err != nil {
		errnie.Error(err)

		return market.Feed{}
	}

	return market.OpenFeed(ctx, public.BalancesChannel, BalanceParams{
		Channel:  public.BalancesChannel,
		Snapshot: true,
		Token:    token,
	})
}
