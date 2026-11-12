package kraken

import (
	"encoding/json"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
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

/*
TradeBalance is private REST/ws account summary — tier hub reads equity and
unrealized here; spendable spot remains on Balance only.
*/
type TradeBalance struct {
	Error    []string           `json:"error"`
	Result   TradeBalanceResult `json:"result"`
	onChange func(market.MarketType)
}

/*
NewTradeBalance prepares an empty wrapper before the first TradeBalance poll.
*/
func NewTradeBalance() *TradeBalance {
	return &TradeBalance{}
}

/*
NewTradeBalanceFromMap adapts paper CLI status JSON into the live wire shape.
*/
func NewTradeBalanceFromMap(model map[string]any) json.Marshaler {
	return &TradeBalance{Result: tradeBalanceFromMap(model)}
}

/*
tradeBalanceFromMap maps paper ledger fields onto Kraken TradeBalanceResult slots.
*/
func tradeBalanceFromMap(model map[string]any) TradeBalanceResult {
	equity := mapDecimal(model["current_value"])
	unrealized := mapDecimal(model["unrealized_pnl"])
	trade := equity.Sub(*unrealized)
	zero := decimal.Zero

	result := TradeBalanceResult{
		AvailableCash:     equity,
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

/*
mapDecimal tolerates CLI float/string decimals without failing paper status polls.
*/
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

/*
Request names ZUSD base for Kraken spot trade-balance summary REST.
*/
func (balance *TradeBalance) Request() map[string]any {
	// Kraken Spot REST: base asset for the summary (default ZUSD).
	return map[string]any{
		"asset": "ZUSD",
	}
}

/*
OnChange wires tier exchange equity display after each decode.
*/
func (balance *TradeBalance) OnChange(fn func(market.MarketType)) {
	balance.onChange = fn
}

/*
MarshalJSON aliases TradeBalance so onChange stays off cached REST payloads.
*/
func (balance *TradeBalance) MarshalJSON() ([]byte, error) {
	type alias TradeBalance
	return sonic.Marshal((*alias)(balance))
}

/*
UnmarshalJSON bumps nonce on rejection and notifies tier after Result decodes.
*/
func (balance *TradeBalance) UnmarshalJSON(data []byte) error {
	type alias TradeBalance
	var decoded alias

	if err := sonic.Unmarshal(data, &decoded); err != nil {
		return err
	}

	if len(decoded.Error) > 0 {
		if balanceNonceRejected(decoded.Error) { // []string
			BumpAuthNonce()
		}

		return errnie.Error(errnie.Err(
			errnie.IO, "[kraken.TradeBalance] venue error in trade-balance frame", nil,
		).With("error", decoded.Error))
	}

	onChange := balance.onChange
	*balance = TradeBalance(decoded)
	balance.onChange = onChange

	if balance.onChange != nil {
		balance.onChange(balance)
	}

	return nil
}

/*
ToMarket projects equity fields into nomagique TradeBalance without treating equity as cash.
*/
func (balance *TradeBalance) ToMarket(other market.MarketType) market.MarketType {
	book, ok := other.(*market.TradeBalance)

	if !ok || book == nil || balance == nil {
		return other
	}

	// TradeBalance is equity/margin accounting only. Spot cash for sizing
	// lives on Balance — never treat equity as spendable here.
	return market.NewTradeBalance(map[string]any{
		"equity":         balance.Result.Equity,
		"unrealized_pnl": balance.Result.UnrealizedPnL,
		"available_cash": balance.Result.AvailableCash,
		"free_margin":    balance.Result.FreeMargin,
	}).ToMarket(book)
}

/*
FromMarket copies hub trade-balance back for paper round-trips.
*/
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
