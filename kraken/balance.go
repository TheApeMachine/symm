package kraken

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
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
NewBalance decodes a streamed balances frame. An undecodable frame is an
error, never a partially filled wallet that would overwrite the account state.
*/
func NewBalance(buf []byte) (*Balance, error) {
	var balance Balance

	if err := sonic.Unmarshal(buf, &balance); err != nil {
		return nil, errnie.Err(
			errnie.UnprocessableContent,
			"[kraken.balance] invalid balances frame",
			err,
		)
	}

	return &balance, nil
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

var zero = decimal.NewFromFloat64(0)

/*
NewBalanceFromMap reshapes the venue's asset-to-total map into the Balance
observation consumed downstream, synthesizing one Data row per asset.
*/
func NewBalanceFromMap(model map[string]*decimal.Decimal) *Balance {
	out := Balance{
		Channel:   "balances",
		Data:      []BalanceData{},
		Type:      "snapshot",
		Sequence:  0,
		Timestamp: time.Now(),
	}

	for asset, amount := range model {
		out.Data = append(out.Data, BalanceData{
			Asset:      asset,
			AssetClass: "currency",
			Balance:    amount,
			Available:  amount,
			Reserved:   zero,
			Wallets: []Wallet{
				{
					Type:    "spot",
					ID:      "main",
					Balance: amount,
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
	var funding *decimal.Decimal

	if amount, found := model["starting_balance"].(float64); found {
		funding = decimal.NewFromFloat64(amount)
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

/*
ToValue serializes wallet balances into a *data.Value layer.
*/
func (balance *Balance) ToValue(epochs ...int64) *data.Value {
	buf, _ := sonic.Marshal(balance)
	val := data.NewValue("kraken", "snapshot", "balances")
	if len(epochs) > 0 {
		val.Epoch = uint64(epochs[0])
		val.FormatPrefix()
	}
	val.Append(buf)
	return val
}

/*
NewBalanceFromValue reconstructs a Balance from a *data.Value layer.
*/
func NewBalanceFromValue(val *data.Value) (*Balance, error) {
	if val == nil || len(val.Layers) == 0 {
		return nil, errnie.Err(errnie.Validation, "empty balance value", nil)
	}
	var balance Balance
	if err := sonic.Unmarshal(val.Layers[0], &balance); err != nil {
		return nil, err
	}
	return &balance, nil
}

