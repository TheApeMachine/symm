package broker

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
)

/*
Cash exposes quote and base availability after live ledger reservations.
It reads wallet rows through Wallet.Get so map access stays under Wallet.mu.
*/
type Cash struct {
	wallet *Wallet
	ledger *Ledger
}

/*
Available reports whether free quote cash covers amount after live reservations.
*/
func (cash *Cash) Available(amount *decimal.Decimal) (bool, error) {
	if err := errnie.Require(map[string]any{
		"amount": amount,
	}); err != nil {
		return false, errnie.Error(err)
	}

	free, err := cash.FreeCash()

	if err != nil {
		return false, err
	}

	return free.Sub(amount).Sign() >= 0, nil
}

/*
FreeCash is exchange quote balance minus open cash reservations on the ledger.
*/
func (cash *Cash) FreeCash() (*decimal.Decimal, error) {
	row, err := cash.wallet.Get(cash.wallet.quote)

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

	if row.Available != nil {
		return row.Available.Copy().Sub(cash.ledger.ReservedCash()), nil
	}

	return row.Balance.Copy().Sub(cash.ledger.ReservedCash()), nil
}

/*
AssetAvailable returns sellable base qty after subtracting sell reservations.
*/
func (cash *Cash) AssetAvailable(asset string) (*decimal.Decimal, error) {
	if err := errnie.Require(map[string]any{
		"asset": asset,
	}); err != nil {
		return nil, errnie.Error(err)
	}

	row, err := cash.wallet.Get(asset)

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

	if row.Available != nil {
		return row.Available.Copy().Sub(cash.ledger.ReservedAsset(asset)), nil
	}

	return row.Balance.Copy().Sub(cash.ledger.ReservedAsset(asset)), nil
}
