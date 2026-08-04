package kraken

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

/*
	PaperBalance is the native payload returned by `kraken paper balance --output json`.

It keeps the full paper wallet rows while exposing a direct Kraken-style decimal
map for callers that only need per-asset totals.
*/
type PaperBalance struct {
	Balances map[string]PaperBalanceData `json:"balances"`
	Mode     string                      `json:"mode"`
}

/*
PaperBalanceData stores the native paper wallet row so available, reserved, and
total amounts survive the CLI decode without float-to-string guesswork later.
*/
type PaperBalanceData struct {
	Available *decimal.Decimal `json:"available"`
	Reserved  *decimal.Decimal `json:"reserved"`
	Total     *decimal.Decimal `json:"total"`
}

type Balance struct {
	Channel   string        `json:"channel"`
	Data      []BalanceData `json:"data"`
	Type      string        `json:"type"`
	Sequence  int64         `json:"sequence"`
	Timestamp time.Time     `json:"timestamp"`
}

type BalanceData struct {
	Asset      string           `json:"asset"`
	AssetClass string           `json:"asset_class"`
	Balance    *decimal.Decimal `json:"balance"`
	Available  *decimal.Decimal `json:"available,omitempty"`
	Reserved   *decimal.Decimal `json:"reserved,omitempty"`
	Wallets    []Wallet         `json:"wallets"`
}

type Wallet struct {
	Type    string           `json:"type"`
	ID      string           `json:"id"`
	Balance *decimal.Decimal `json:"balance"`
}

/*
BalanceSubscription requests the authenticated wallet stream through the same
transport abstraction used by public market subscriptions.
*/
type BalanceSubscription struct {
	Token string
}

/*
NewBalanceSubscription binds the current authenticated websocket token.
*/
func NewBalanceSubscription(token string) BalanceSubscription {
	return BalanceSubscription{Token: token}
}

/*
MarshalJSON encodes Kraken's private balances subscription request.
*/
func (subscription BalanceSubscription) MarshalJSON() ([]byte, error) {
	return sonic.Marshal(map[string]any{
		"method": "subscribe",
		"params": map[string]any{
			"channel": "balances",
			"token":   subscription.Token,
		},
	})
}

func NewBalance(buf []byte) *Balance {
	var balance Balance

	if err := sonic.Unmarshal(buf, &balance); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"invalid balance",
			err,
		))
	}

	return &balance
}

func (balance *Balance) MarshalJSON() ([]byte, error) {
	type alias Balance
	return sonic.Marshal((*alias)(balance))
}

func (balance *Balance) Action() string {
	return "balance"
}

func (balance *Balance) IsSuccess() bool {
	return len(balance.Data) > 0
}

/*
NewPaperBalance decodes the native paper wallet payload into Kraken-style asset
totals so callers can consume paper and real balances through the same map type.
*/
func NewPaperBalance(buf []byte) *PaperBalance {
	var balance PaperBalance

	if err := sonic.Unmarshal(buf, &balance); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"invalid paper balance",
			err,
		))
	}

	return &balance
}

/*
Totals returns the same asset-to-decimal total map shape produced by Kraken's
real balance endpoint, using each paper wallet row's total amount.
*/
func (balance *PaperBalance) Totals() map[string]*decimal.Decimal {
	totals := make(map[string]*decimal.Decimal, len(balance.Balances))

	for asset, data := range balance.Balances {
		totals[asset] = data.Total
	}

	return totals
}

/*
NewBalanceFromMap reshapes the paper CLI wallet dump into the websocket balance
frame used by downstream consumers, preserving available, reserved, and total
values for each asset row.
*/
func NewBalanceFromMap(model datura.Map[any]) *Balance {
	out := Balance{
		Channel:   "balances",
		Data:      []BalanceData{},
		Type:      "snapshot",
		Sequence:  0,
		Timestamp: time.Now(),
	}

	balances, ok := model["balances"].(map[string]any)

	if !ok {
		return &out
	}

	for asset, entryRaw := range balances {
		entry, ok := entryRaw.(map[string]any)

		if !ok {
			continue
		}

		available := decimal.NewFromFloat64(entry["available"].(float64))
		reserved := decimal.NewFromFloat64(entry["reserved"].(float64))
		total := decimal.NewFromFloat64(entry["total"].(float64))

		out.Data = append(out.Data, BalanceData{
			Asset:      asset,
			AssetClass: "currency",
			Balance:    total,
			Available:  available,
			Reserved:   reserved,
			Wallets: []Wallet{
				{
					Type:    "spot",
					ID:      "main",
					Balance: total,
				},
			},
		})
	}

	return &out
}

func NewTradeBalanceFromMap(model datura.Map[any]) TradeBalanceResult {
	currentValue := decimal.NewFromFloat64(model["current_value"].(float64))
	unrealizedPnL := decimal.NewFromFloat64(model["unrealized_pnl"].(float64))
	tradeBalance := currentValue.Sub(unrealizedPnL)
	zero := decimal.NewFromInt64(0)

	return TradeBalanceResult{
		EquivalentBalance: currentValue.Copy(),
		TradeBalance:      tradeBalance,
		MarginAmount:      zero.Copy(),
		UnrealizedPnL:     unrealizedPnL,
		CostBasis:         zero.Copy(),
		Valuation:         zero.Copy(),
		Equity:            currentValue,
		FreeMargin:        currentValue.Copy(),
		MarginFreeOrders:  currentValue.Copy(),
		UnexecutedValue:   zero.Copy(),
	}
}
