package kraken

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
TradeBalanceResult mirrors Kraken's private `/0/private/TradeBalance` result.
*/
type TradeBalanceResult struct {
	AvailableCash     *decimal.Decimal `json:"-"`
	NetFunding        *decimal.Decimal `json:"-"`
	FundingReason     string           `json:"-"`
	ValuationComplete *bool            `json:"-"`
	EquivalentBalance *decimal.Decimal `json:"eb"`
	TradeBalance      *decimal.Decimal `json:"tb"`
	MarginAmount      *decimal.Decimal `json:"m"`
	UnrealizedPnL     *decimal.Decimal `json:"n"`
	CostBasis         *decimal.Decimal `json:"c"`
	Valuation         *decimal.Decimal `json:"v"`
	Equity            *decimal.Decimal `json:"e"`
	FreeMargin        *decimal.Decimal `json:"mf"`
	MarginFreeOrders  *decimal.Decimal `json:"mfo,omitempty"`
	MarginLevel       *decimal.Decimal `json:"ml,omitempty"`
	UnexecutedValue   *decimal.Decimal `json:"uv,omitempty"`
}

type TradeBalance struct {
	Error  []string           `json:"error"`
	Result TradeBalanceResult `json:"result"`
}

