package kraken

import (
	"encoding/json"
	"time"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
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

/*
BalanceSubscription requests authenticated wallet snapshots.
*/
type BalanceSubscription struct {
	Token string
}

func NewBalanceSubscription(token string) BalanceSubscription {
	return BalanceSubscription{Token: token}
}

func (subscription BalanceSubscription) MarshalJSON() ([]byte, error) {
	params := map[string]any{
		"channel":  "balances",
		"snapshot": true,
	}

	if subscription.Token != "" {
		params["token"] = subscription.Token
	}

	return sonic.Marshal(map[string]any{
		"method": "subscribe",
		"params": params,
	})
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

func (balance *Balance) MarshalJSON() ([]byte, error) {
	type alias Balance
	return sonic.Marshal((*alias)(balance))
}

func (balance *Balance) UnmarshalJSON(data []byte) error {
	type alias Balance
	var decoded alias

	if err := sonic.Unmarshal(data, &decoded); err != nil {
		return err
	}

	*balance = Balance(decoded)
	return nil
}

func (balance *Balance) Action() string {
	return "balance"
}

func (balance *Balance) IsSuccess() bool {
	return len(balance.Data) > 0
}

/*
NewPaperBalance decodes the native paper wallet payload into the same Balance
observation shape as the real REST endpoint, keeping each row's available,
reserved, and total amounts as separate Data fields.
*/
func NewPaperBalance(buf []byte) *Balance {
	var paper PaperBalance

	if err := sonic.Unmarshal(buf, &paper); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"invalid paper balance",
			err,
		))
	}

	out := Balance{
		Channel:   "balances",
		Data:      []BalanceData{},
		Type:      "snapshot",
		Timestamp: time.Now(),
	}

	for asset, row := range paper.Balances {
		out.Data = append(out.Data, BalanceData{
			Asset:      asset,
			AssetClass: "currency",
			Balance:    row.Total,
			Available:  row.Available,
			Reserved:   row.Reserved,
			Wallets: []Wallet{
				{
					Type:    "spot",
					ID:      "main",
					Balance: row.Total,
				},
			},
		})
	}

	return &out
}

var zero = dFloat(0)

/*
NewBalanceFromMap reshapes the venue's asset-to-total map into the Balance
observation consumed downstream, synthesizing one Data row per asset.
*/
func NewBalanceFromMap(model map[string]any) json.Marshaler {
	if _, ok := model["balances"]; ok {
		raw, err := sonic.Marshal(model)

		if err != nil {
			return &Balance{Channel: "balances", Type: "snapshot", Timestamp: time.Now()}
		}

		return NewPaperBalance(raw)
	}

	out := Balance{
		Channel:   "balances",
		Data:      []BalanceData{},
		Type:      "snapshot",
		Sequence:  0,
		Timestamp: time.Now(),
	}

	for asset, amount := range model {
		parsed, ok := amount.(*decimal.Decimal)

		if !ok {
			continue
		}

		out.Data = append(out.Data, BalanceData{
			Asset:      asset,
			AssetClass: "currency",
			Balance:    parsed,
			Available:  parsed,
			Reserved:   zero,
			Wallets: []Wallet{
				{
					Type:    "spot",
					ID:      "main",
					Balance: parsed,
				},
			},
		})
	}

	return &out
}

func NewTradeBalanceFromMap(model map[string]any) TradeBalanceResult {
	currentValue := dFloat(floatField(model["current_value"]))
	unrealizedPnL := dFloat(floatField(model["unrealized_pnl"]))
	tradeBalanceVal := currentValue.Sub(*unrealizedPnL)
	tradeBalance := &tradeBalanceVal

	zero := dInt(0)
	var funding *decimal.Decimal

	if amount, found := model["starting_balance"].(float64); found {
		funding = dFloat(amount)
	}

	complete, known := model["valuation_complete"].(bool)

	var valuation *bool

	if known {
		valuation = &complete
	}

	return TradeBalanceResult{
		NetFunding:        funding,
		ValuationComplete: valuation,
		EquivalentBalance: currentValue,
		TradeBalance:      tradeBalance,
		MarginAmount:      zero,
		UnrealizedPnL:     unrealizedPnL,
		CostBasis:         zero,
		Valuation:         zero,
		Equity:            currentValue,
		FreeMargin:        currentValue,
		MarginFreeOrders:  currentValue,
		UnexecutedValue:   zero,
	}
}
