package kraken

import (
	"encoding/json"
	"strings"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

/*
ExtendedBalance retains exchange holds separately from owned spot cash. Unused
credit is not owned spot capital. Protocol quantities come from BalanceEx:
https://docs.kraken.com/api-reference/account-data/get-extended-balance
*/
type ExtendedBalance struct {
	Error  []string `json:"error"`
	Result map[string]struct {
		Balance *decimal.Decimal `json:"balance"`
		Hold    *decimal.Decimal `json:"hold_trade"`
		Used    *decimal.Decimal `json:"credit_used"`
	} `json:"result"`
}

/* NewExtendedBalance rejects absent or malformed full-account responses. */
func NewExtendedBalance(payload []byte) (*ExtendedBalance, error) {
	var balance ExtendedBalance

	if err := json.Unmarshal(payload, &balance); err != nil {
		return nil, errnie.Err(errnie.Validation, "extended balance: invalid response", err)
	}

	if len(balance.Error) > 0 || balance.Result == nil {
		return nil, errnie.Err(errnie.Validation, "extended balance: unavailable account: "+strings.Join(balance.Error, "; "), nil)
	}
	return &balance, nil
}

/* Available subtracts exchange holds and used credit from the owned quote balance. */
func (balance *ExtendedBalance) Available(quote string, normalize func(string) string) (*decimal.Decimal, error) {
	var available *decimal.Decimal
	for asset, row := range balance.Result {
		if normalize(asset) != quote {
			continue
		}

		if available != nil || row.Balance == nil || row.Hold == nil {
			return nil, errnie.Err(errnie.Validation, "extended balance: unique complete quote row required", nil)
		}
		available = row.Balance.SetScale(max(row.Balance.GetScale(), row.Hold.GetScale())).Sub(row.Hold)

		if row.Used != nil {
			available = available.SetScale(max(available.GetScale(), row.Used.GetScale())).Sub(row.Used)
		}
	}
	// The endpoint is a complete asset map: absence means no holding of that asset.
	if available == nil {
		return decimal.NewFromInt64(0), nil
	}
	return available, nil
}
