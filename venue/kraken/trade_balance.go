package kraken

import (
	"encoding/json"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
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

type TradeBalanceRequest struct {
	Asset string `json:"asset,omitempty"`
}

func NewTradeBalanceRequest(asset string) *TradeBalanceRequest {
	return &TradeBalanceRequest{Asset: asset}
}

func (request *TradeBalanceRequest) MarshalJSON() ([]byte, error) {
	type alias TradeBalanceRequest
	return sonic.Marshal((*alias)(request))
}

func NewTradeBalanceEnvelopeFromMap(model map[string]any) json.Marshaler {
	return &TradeBalance{Result: NewTradeBalanceFromMap(model)}
}

func (balance *TradeBalance) MarshalJSON() ([]byte, error) {
	type alias TradeBalance
	return sonic.Marshal((*alias)(balance))
}

func NewTradeBalance(buf []byte) TradeBalanceResult {
	balance := TradeBalance{}

	if err := sonic.Unmarshal(buf, &balance); err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent, "invalid trade balance", err,
		))
		return TradeBalanceResult{}
	}

	if len(balance.Error) > 0 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"trade balance response contains errors: "+strings.Join(balance.Error, "; "),
			nil,
		))
		return TradeBalanceResult{}
	}

	return balance.Result
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
