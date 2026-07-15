package types

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken"
)

/*
Holding records either wallet-backed inventory or a Thesis-local candidate.
Position orchestrates live holdings; a candidate remains on its originating
Thesis even when strategy declines to submit its proposed order.
*/
type Holding struct {
	Request    *spot.OrderRequest  `json:"request,omitempty"`
	Order      *spot.Order         `json:"order,omitempty"`
	Executions []*kraken.Execution `json:"executions,omitempty"`
	Symbol     string              `json:"symbol"`
	Asset      string              `json:"asset,omitempty"`
	Qty        decimal.Decimal     `json:"qty"`
	EntryAt    time.Time           `json:"entry_at,omitempty"`
	ExitAt     time.Time           `json:"exit_at,omitempty"`
	EntryPrice decimal.Decimal     `json:"entry_price"`
	EntryFee   decimal.Decimal     `json:"entry_fee"`
	ExitPrice  decimal.Decimal     `json:"exit_price"`
	ExitFee    decimal.Decimal     `json:"exit_fee"`
	PnL        decimal.Decimal     `json:"pnl"`
	ReturnPct  float64             `json:"return_pct"`
	Mark       decimal.Decimal     `json:"mark"`
}
