package broker

import (
	"maps"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

/*
Available reports whether free quote cash covers amount after live reservations.
*/
func (balance *Balance) Available(amount *decimal.Decimal) bool {
	if balance.validate(map[string]any{
		"amount": amount,
	}) != nil {
		return false
	}

	free, err := balance.FreeCash()

	if err != nil {
		errnie.Error(err)

		return false
	}

	return free.Sub(amount).Sign() >= 0
}

/*
FreeCash is exchange quote balance minus open cash reservations.
*/
func (balance *Balance) FreeCash() (*decimal.Decimal, error) {
	row, err := balance.Get(balance.quote)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"quote balance not found",
			err,
		))
	}

	if row.Balance == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"quote balance not found",
			nil,
		))
	}

	reserved := balance.ledger.ReservedCash()

	return row.Balance.Copy().Sub(reserved), nil
}

/*
AssetAvailable returns sellable base qty after subtracting sell reservations.
*/
func (balance *Balance) AssetAvailable(asset string) (*decimal.Decimal, error) {
	if err := balance.validate(map[string]any{
		"asset": asset,
	}); err != nil {
		return nil, err
	}

	row, err := balance.Get(asset)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"asset available balance missing for "+asset,
			err,
		))
	}

	if row.Balance == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"asset available balance missing for "+asset,
			nil,
		))
	}

	reserved := balance.ledger.ReservedAsset(asset)

	return row.Balance.Copy().Sub(reserved), nil
}

func (balance *Balance) validate(mandatory map[string]any) error {
	check := map[string]any{
		"data": balance.data,
	}

	maps.Copy(check, mandatory)

	return errnie.Error(errnie.Require(check))
}
