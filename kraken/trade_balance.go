package kraken

import (
	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
)

/*
TradeBalanceResult mirrors Kraken's private `/0/private/TradeBalance` result.
*/
type TradeBalanceResult struct {
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

/*
TradeBalanceRequest requests Kraken trade balance in one base asset.
*/
type TradeBalanceRequest struct {
	Asset string `json:"asset,omitempty"`
}

/*
NewTradeBalance parses one Kraken trade balance response.
*/
func NewTradeBalance(buf []byte) *TradeBalanceResult {
	balance := &TradeBalance{}

	if err := sonic.Unmarshal(buf, balance); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"invalid trade balance",
			err,
		))

		return nil
	}

	return &balance.Result
}

/*
NewTradeBalanceRequest binds the asset basis for Kraken trade balance.
*/
func NewTradeBalanceRequest(asset string) *TradeBalanceRequest {
	return &TradeBalanceRequest{Asset: asset}
}

func (request *TradeBalanceRequest) MarshalJSON() ([]byte, error) {
	type alias TradeBalanceRequest
	return sonic.Marshal((*alias)(request))
}

/*
NewPaperTradeBalanceFromMap reshapes `kraken paper status --verbose` into the
Kraken trade-balance response shape. The liquidation value lives in `e`.
*/
func NewPaperTradeBalanceFromMap(model datura.Map[any]) *TradeBalanceResult {
	currentValue, _ := model["current_value"].(float64)
	unrealizedPnL, _ := model["unrealized_pnl"].(float64)
	equity := decimal.NewFromFloat64(currentValue)
	unrealized := decimal.NewFromFloat64(unrealizedPnL)
	tradeBalance := equity.Sub(unrealized)

	return &TradeBalanceResult{
		EquivalentBalance: equity.Copy(),
		TradeBalance:      tradeBalance,
		MarginAmount:      decimal.NewFromInt64(0),
		UnrealizedPnL:     unrealized,
		CostBasis:         decimal.NewFromInt64(0),
		Valuation:         decimal.NewFromInt64(0),
		Equity:            equity,
		FreeMargin:        equity.Copy(),
		MarginFreeOrders:  equity.Copy(),
		UnexecutedValue:   decimal.NewFromInt64(0),
	}
}
