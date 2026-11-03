package kraken

import (
	"encoding/json"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/nomagique/market"
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
	Error    []string           `json:"error"`
	Result   TradeBalanceResult `json:"result"`
	onChange func(market.MarketType)
}

func NewTradeBalance() *TradeBalance {
	return &TradeBalance{}
}

func NewTradeBalanceFromMap(model map[string]any) json.Marshaler {
	return &TradeBalance{Result: tradeBalanceFromMap(model)}
}

func tradeBalanceFromMap(model map[string]any) TradeBalanceResult {
	equity := mapDecimal(model["current_value"])
	unrealized := mapDecimal(model["unrealized_pnl"])
	trade := equity.Sub(*unrealized)
	zero := decimal.Zero

	result := TradeBalanceResult{
		EquivalentBalance: equity,
		TradeBalance:      &trade,
		MarginAmount:      &zero,
		UnrealizedPnL:     unrealized,
		CostBasis:         &zero,
		Valuation:         &zero,
		Equity:            equity,
		FreeMargin:        equity,
		MarginFreeOrders:  equity,
		UnexecutedValue:   &zero,
	}

	if amount, ok := model["starting_balance"].(float64); ok {
		funding := decimal.NewFromFloat(amount)
		result.NetFunding = &funding
	}

	if complete, ok := model["valuation_complete"].(bool); ok {
		result.ValuationComplete = &complete
	}

	return result
}

func mapDecimal(raw any) *decimal.Decimal {
	switch value := raw.(type) {
	case float64:
		out := decimal.NewFromFloat(value)
		return &out
	case string:
		out, err := decimal.NewFromString(value)
		if err != nil {
			zero := decimal.Zero
			return &zero
		}
		return &out
	default:
		zero := decimal.Zero
		return &zero
	}
}

func (balance *TradeBalance) Request() map[string]any {
	return map[string]any{
		"nonce": 0,
	}
}

func (balance *TradeBalance) OnChange(fn func(market.MarketType)) {
	balance.onChange = fn
}

func (balance *TradeBalance) MarshalJSON() ([]byte, error) {
	type alias TradeBalance
	return sonic.Marshal((*alias)(balance))
}

func (balance *TradeBalance) UnmarshalJSON(data []byte) error {
	type alias TradeBalance
	var decoded alias

	if err := sonic.Unmarshal(data, &decoded); err != nil {
		return err
	}

	onChange := balance.onChange
	*balance = TradeBalance(decoded)
	balance.onChange = onChange

	if balance.onChange != nil {
		balance.onChange(balance)
	}

	return nil
}

func (balance *TradeBalance) ToMarket(other market.MarketType) market.MarketType {
	book, ok := other.(*market.TradeBalance)

	if !ok || book == nil || balance == nil {
		return other
	}

	return market.NewTradeBalance(map[string]any{
		"equity":         balance.Result.Equity,
		"unrealized_pnl": balance.Result.UnrealizedPnL,
		"available_cash": balance.Result.AvailableCash,
		"free_margin":    balance.Result.FreeMargin,
	}).ToMarket(book)
}

func (balance *TradeBalance) FromMarket(other market.MarketType) market.MarketType {
	book, ok := other.(*market.TradeBalance)

	if !ok || book == nil || balance == nil {
		return balance
	}

	balance.Result.Equity = book.Equity
	balance.Result.UnrealizedPnL = book.UnrealizedPnL
	balance.Result.AvailableCash = book.AvailableCash
	balance.Result.FreeMargin = book.FreeMargin

	return balance
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
