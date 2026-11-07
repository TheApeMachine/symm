package kraken

import (
	"encoding/json"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
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
	out.Channel = "balances"
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

type balanceWallet struct {
	Type    string          `json:"type"`
	ID      string          `json:"id"`
	Balance decimal.Decimal `json:"balance"`
}

type balanceRow struct {
	Asset      string          `json:"asset"`
	Balance    decimal.Decimal `json:"balance"`
	Amount     decimal.Decimal `json:"amount"`
	Wallets    []balanceWallet `json:"wallets"`
	WalletType string          `json:"wallet_type"`
	WalletID   string          `json:"wallet_id"`
}

type balanceFrame struct {
	Channel string                      `json:"channel"`
	Type    string                      `json:"type"`
	Error   []any                       `json:"error"`
	Result  map[string]*decimal.Decimal `json:"result"`
	Data    []balanceRow                `json:"data"`
}

func (balance *Balance) UnmarshalJSON(data []byte) error {
	var decoded balanceFrame

	if err := sonic.Unmarshal(data, &decoded); err != nil {
		return err
	}

	if len(decoded.Error) > 0 {
		return errnie.Error(errnie.Err(
			errnie.IO, "[kraken.Balance] venue error in balance frame", nil,
		).With("error", decoded.Error))
	}

	onChange := balance.onChange

	if decoded.Channel == "balances" && len(decoded.Data) > 0 {
		balance.fold(decoded)
	} else {
		*balance = Balance{
			Channel: decoded.Channel,
			Error:   decoded.Error,
			Result:  decoded.Result,
		}
	}

	balance.onChange = onChange

	if balance.Result == nil {
		balance.Result = make(map[string]*decimal.Decimal)
	}

	if balance.onChange != nil {
		balance.onChange(balance)
	}

	return nil
}

/*
fold copies a private balances snapshot or update into Result.
Spendable cash is the spot main wallet, not earn.
*/
func (balance *Balance) fold(frame balanceFrame) {
	updating := frame.Type == "update" && balance.Result != nil

	if !updating {
		balance.Result = make(map[string]*decimal.Decimal)
	}

	balance.Channel = "balances"

	for _, row := range frame.Data {
		amount, ok := row.spendable()

		if !ok {
			continue
		}

		// Update `balance` is the account total. Keep the spot wallet by
		// applying the spot delta when we already have one.
		if updating && row.WalletType == "spot" {
			if prev := balance.Result[row.Asset]; prev != nil {
				amount = prev.Add(row.Amount)
			}
		}

		next := amount
		balance.Result[row.Asset] = &next
	}
}

/*
spendable is the spot main balance the desk can allocate.
*/
func (row balanceRow) spendable() (decimal.Decimal, bool) {
	if row.Asset == "" {
		return decimal.Zero, false
	}

	if row.WalletType != "" {
		if row.WalletType != "spot" {
			return decimal.Zero, false
		}

		if row.WalletID != "" && row.WalletID != "main" {
			return decimal.Zero, false
		}

		return row.Balance, true
	}

	for _, wallet := range row.Wallets {
		if wallet.Type == "spot" && (wallet.ID == "main" || wallet.ID == "") {
			return wallet.Balance, true
		}
	}

	if len(row.Wallets) == 0 {
		return row.Balance, true
	}

	return decimal.Zero, false
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
