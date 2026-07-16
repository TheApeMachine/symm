package kraken

import (
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

type Balance struct {
	Channel   string        `json:"channel"`
	Data      []BalanceData `json:"data"`
	Type      string        `json:"type"`
	Sequence  int64         `json:"sequence"`
	Timestamp time.Time     `json:"timestamp"`
}

type BalanceData struct {
	LedgerID   string           `json:"ledger_id"`
	RefID      string           `json:"ref_id"`
	Timestamp  time.Time        `json:"timestamp"`
	Type       string           `json:"type"`
	Subtype    string           `json:"subtype"`
	Asset      string           `json:"asset"`
	AssetClass string           `json:"asset_class"`
	Category   string           `json:"category"`
	WalletType string           `json:"wallet_type"`
	WalletID   string           `json:"wallet_id"`
	Amount     *decimal.Decimal `json:"amount"`
	Fee        *decimal.Decimal `json:"fee"`
	Balance    *decimal.Decimal `json:"balance"`
	Available  *decimal.Decimal `json:"available"`
	Reserved   *decimal.Decimal `json:"reserved"`
	User       string           `json:"user"`
	Wallets    []Wallet         `json:"wallets"`
}

type Wallet struct {
	Type    string          `json:"type"`
	ID      string          `json:"id"`
	Balance *decimal.Decimal `json:"balance"`
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

		total := decimal.NewFromFloat64(entry["total"].(float64))

		out.Data = append(out.Data, BalanceData{
			Asset:      asset,
			AssetClass: "currency",
			Available:  decimal.NewFromFloat64(entry["available"].(float64)),
			Balance:    total,
			Reserved:   decimal.NewFromFloat64(entry["reserved"].(float64)),
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
