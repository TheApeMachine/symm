package user

import (
	"context"

	"github.com/bytedance/sonic"
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
*/
type Balance struct {
	Asset      string          `json:"asset"`
	AssetClass string          `json:"asset_class"`
	Balance    float64         `json:"balance"`
	Wallets    []BalanceWallet `json:"wallets,omitempty"`
	Amount     float64         `json:"amount,omitempty"`
	Fee        float64         `json:"fee,omitempty"`
	LedgerID   string          `json:"ledger_id,omitempty"`
	RefID      string          `json:"ref_id,omitempty"`
	Timestamp  string          `json:"timestamp,omitempty"`
	Type       string          `json:"type,omitempty"`
	Subtype    string          `json:"subtype,omitempty"`
	Category   string          `json:"category,omitempty"`
	WalletType string          `json:"wallet_type,omitempty"`
	WalletID   string          `json:"wallet_id,omitempty"`
	User       string          `json:"user,omitempty"`
	Envelope   string          `json:"-"`
}

func (balance *Balance) SetEnvelopeType(kind string) {
	balance.Envelope = kind
}

func (balance *Balance) IsSnapshot() bool {
	return balance.Envelope == balanceSnapshot
}

/*
DecodeBalance decodes one balances data row after SplitDataRows.
*/
func DecodeBalance(row *public.SocketMessage) (Balance, error) {
	var balance Balance

	if err := sonic.Unmarshal(row.Data, &balance); err != nil {
		return Balance{}, err
	}

	balance.SetEnvelopeType(row.Type)

	return balance, nil
}

/*
DecodeBalances decodes every row in a balances channel message.
*/
func DecodeBalances(message *public.SocketMessage) ([]Balance, error) {
	var rows []Balance

	if err := sonic.Unmarshal(message.Data, &rows); err != nil {
		return nil, err
	}

	for index := range rows {
		rows[index].SetEnvelopeType(message.Type)
	}

	return rows, nil
}
