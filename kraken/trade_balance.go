package kraken

import (
	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
)

/*
TradeBalanceResult mirrors Kraken's private `/0/private/TradeBalance` result.
*/
type TradeBalanceResult struct {
	AvailableCash     *decimal.Decimal `json:"available_cash,omitempty"`
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

/*
AssetAlias maps venue legacy currency prefixes to standard codes and vice versa.
*/
func AssetAlias(asset string) string {
	switch asset {
	case "USD":
		return "ZUSD"
	case "ZUSD":
		return "USD"
	case "EUR":
		return "ZEUR"
	case "ZEUR":
		return "EUR"
	case "GBP":
		return "ZGBP"
	case "ZGBP":
		return "GBP"
	case "CAD":
		return "ZCAD"
	case "ZCAD":
		return "CAD"
	case "JPY":
		return "ZJPY"
	case "ZJPY":
		return "JPY"
	case "BTC":
		return "XXBT"
	case "XXBT":
		return "BTC"
	case "XBT":
		return "BTC"
	case "ETH":
		return "XETH"
	case "XETH":
		return "ETH"
	default:
		return ""
	}
}

/*
ToValue serializes the TradeBalanceResult and associated wallet into a *data.Value layer.
*/
func (tb *TradeBalanceResult) ToValue(wallet *Balance, quote string, epochs ...int64) *data.Value {
	if tb == nil {
		return nil
	}

	val := data.NewValue("kraken", "trade_balance", quote)
	if len(epochs) > 0 && epochs[0] > 0 {
		val.Epoch = uint64(epochs[0])
	}
	val.Append(tb)
	if wallet != nil {
		val.Append(wallet)
	}
	return val
}

/*
NewTradeBalanceResultFromValue reconstructs a TradeBalanceResult from a *data.Value layer.
*/
func NewTradeBalanceResultFromValue(val *data.Value) (*TradeBalanceResult, error) {
	if val == nil || len(val.Layers) == 0 {
		return nil, errnie.Err(errnie.Validation, "[kraken.trade_balance] cannot convert nil value to trade balance", nil)
	}

	var tb TradeBalanceResult
	if err := sonic.Unmarshal(val.Layers[0], &tb); err != nil {
		return nil, errnie.Err(errnie.Validation, "[kraken.trade_balance] failed to unmarshal trade balance", err)
	}

	return &tb, nil
}

