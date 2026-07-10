package kraken

import (
	"sort"
	"time"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
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
	LedgerID   string          `json:"ledger_id"`
	RefID      string          `json:"ref_id"`
	Timestamp  time.Time       `json:"timestamp"`
	Type       string          `json:"type"`
	Subtype    string          `json:"subtype"`
	Asset      string          `json:"asset"`
	AssetClass string          `json:"asset_class"`
	Category   string          `json:"category"`
	WalletType string          `json:"wallet_type"`
	WalletID   string          `json:"wallet_id"`
	Amount     decimal.Decimal `json:"amount"`
	Fee        decimal.Decimal `json:"fee"`
	Balance    decimal.Decimal `json:"balance"`
	Available  decimal.Decimal `json:"available"`
	Reserved   decimal.Decimal `json:"reserved"`
	User       string          `json:"user"`
	Wallets    []struct {
		Type    string          `json:"type"`
		ID      string          `json:"id"`
		Balance decimal.Decimal `json:"balance"`
	} `json:"wallets"`
}

type BalanceDataSlice []BalanceData

func NewBalanceDataSlice(buf []byte) *BalanceDataSlice {
	frame := Balance{}

	if err := sonic.Unmarshal(buf, &frame); err == nil && frame.Channel == "balances" {
		data := BalanceDataSlice(frame.Data)
		return &data
	}

	data := &BalanceDataSlice{}
	errnie.Error(sonic.Unmarshal(buf, data))

	return data
}

func NewBalanceDataSliceFromSpot(
	balances map[string]*decimal.Decimal,
) BalanceDataSlice {
	assets := make([]string, 0, len(balances))

	for asset := range balances {
		assets = append(assets, asset)
	}

	sort.Strings(assets)
	rows := make(BalanceDataSlice, 0, len(assets))

	for _, asset := range assets {
		if balances[asset] == nil {
			continue
		}

		rows = append(rows, BalanceData{
			Asset:      asset,
			AssetClass: "currency",
			Balance:    *balances[asset],
			Available:  *balances[asset],
		})
	}

	return rows
}
