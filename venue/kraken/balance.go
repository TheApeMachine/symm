package kraken

import (
	"encoding/json"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/nomagique/market"
)

type Balance struct {
	Channel  string                      `json:"channel,omitempty"`
	Error    []any                       `json:"error"`
	Result   map[string]*decimal.Decimal `json:"result"`
	onChange func(market.MarketType)
}

func NewBalance() *Balance {
	return &Balance{
		Result: make(map[string]*decimal.Decimal),
	}
}

/*
NewBalanceFromMap builds a REST Balance from the paper CLI balance model.
*/
func NewBalanceFromMap(model map[string]any) json.Marshaler {
	out := NewBalance()
	rows, _ := model["balances"].(map[string]any)

	for asset, raw := range rows {
		row, _ := raw.(map[string]any)
		amount := decimal.Zero

		if value, ok := row["available"].(float64); ok {
			amount = decimal.NewFromFloat(value)
		} else if value, ok := row["total"].(float64); ok {
			amount = decimal.NewFromFloat(value)
		}

		if amount.IsZero() {
			continue
		}

		capital := amount
		out.Result[asset] = &capital
	}

	return out
}

func (balance *Balance) Request() map[string]any {
	return map[string]any{
		"nonce": 0,
	}
}

func (balance *Balance) OnChange(fn func(market.MarketType)) {
	balance.onChange = fn
}

func (balance *Balance) MarshalJSON() ([]byte, error) {
	type alias Balance
	return sonic.Marshal((*alias)(balance))
}

func (balance *Balance) UnmarshalJSON(data []byte) error {
	type alias Balance
	var decoded alias

	if err := sonic.Unmarshal(data, &decoded); err != nil {
		return err
	}

	onChange := balance.onChange
	*balance = Balance(decoded)
	balance.onChange = onChange

	if balance.Result == nil {
		balance.Result = make(map[string]*decimal.Decimal)
	}

	if balance.onChange != nil {
		balance.onChange(balance)
	}

	return nil
}

func (balance *Balance) ToMarket(other market.MarketType) market.MarketType {
	book, ok := other.(*market.Balance)

	if !ok || book == nil || balance == nil {
		return other
	}

	assets := make([]*market.Asset, 0, len(balance.Result))

	for asset, amount := range balance.Result {
		if amount == nil {
			continue
		}

		assets = append(assets, market.NewAsset(
			nil, asset, *amount, decimal.Zero, *amount,
		))
	}

	return market.NewBalance(map[string]any{
		"quote":  book.Quote,
		"assets": assets,
	}).ToMarket(book)
}

func (balance *Balance) FromMarket(other market.MarketType) market.MarketType {
	book, ok := other.(*market.Balance)

	if !ok || book == nil || balance == nil {
		return balance
	}

	if balance.Result == nil {
		balance.Result = make(map[string]*decimal.Decimal)
	}

	clear(balance.Result)

	for _, asset := range book.Assets {
		if asset == nil {
			continue
		}

		capital := asset.Capital
		balance.Result[asset.Symbol] = &capital
	}

	return balance
}
