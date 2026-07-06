package kraken

import (
	"time"

	"github.com/bytedance/sonic"
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
	LedgerID   string    `json:"ledger_id"`
	RefID      string    `json:"ref_id"`
	Timestamp  time.Time `json:"timestamp"`
	Type       string    `json:"type"`
	Subtype    string    `json:"subtype"`
	Asset      string    `json:"asset"`
	AssetClass string    `json:"asset_class"`
	Category   string    `json:"category"`
	WalletType string    `json:"wallet_type"`
	WalletID   string    `json:"wallet_id"`
	Amount     float64   `json:"amount"`
	Fee        float64   `json:"fee"`
	Balance    float64   `json:"balance"`
	Available  float64   `json:"available"`
	Reserved   float64   `json:"reserved"`
	User       string    `json:"user"`
	Wallets    []struct {
		Type    string  `json:"type"`
		ID      string  `json:"id"`
		Balance float64 `json:"balance"`
	} `json:"wallets"`
}

type BalanceDataSlice []BalanceData

func NewBalanceDataSlice(buf []byte) *BalanceDataSlice {
	data := &BalanceDataSlice{}
	errnie.Error(sonic.Unmarshal(buf, data))
	return data
}
