package trader

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/types"
)

/*
Plan seeds open inventory, runs Planner.Decide, and publishes strategy frames.
*/
func (crypto *Crypto) Plan(thesis *types.Thesis) error {
	if thesis == nil || crypto.planner == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"crypto: plan requires thesis and planner",
			nil,
		))
	}

	crypto.seedOpen(thesis)
	crypto.planner.Decide(thesis)
	crypto.publishStrategy(thesis)

	return nil
}

/*
seedOpen copies broker-open inventory onto Thesis.Holdings so Decide's
continuation branch sees real positions, then merges durable recovery state
back onto Balance so Desk can adopt sellable shells.
*/
func (crypto *Crypto) seedOpen(thesis *types.Thesis) {
	if crypto.balance != nil {
		for holding := range crypto.balance.Holdings() {
			seed := holding
			thesis.Holdings.Store(holding.Symbol, &seed)
		}
	}

	if crypto.recovery != nil && crypto.recovery.Holdings != nil {
		crypto.recovery.Holdings.Range(func(key, value any) bool {
			if _, exists := thesis.Holdings.Load(key); exists {
				return true
			}

			switch holding := value.(type) {
			case *types.Holding:
				thesis.Holdings.Store(key, holding)
				crypto.restoreHolding(holding)
			case types.Holding:
				seed := holding
				thesis.Holdings.Store(key, &seed)
				crypto.restoreHolding(&seed)
			}

			return true
		})

		// One-shot: never reintroduce recovered lots after the first seed.
		crypto.recovery = nil
	}

	if crypto.desk != nil {
		_ = crypto.desk.OpenPositions()
	}
}

/*
restoreHolding puts a recovered open lot onto Balance when the wallet map lost
the shell after a partial-exit bug or restart race.
*/
func (crypto *Crypto) restoreHolding(holding *types.Holding) {
	if crypto.balance == nil {
		return
	}

	crypto.balance.Remember(holding)
}

/*
constraints returns quote capital plus normal and reserved slot ceilings.
*/
func (crypto *Crypto) constraints() (float64, int, int, error) {
	normal, reserved := 0, 0

	if crypto.desk != nil {
		normal = crypto.desk.NormalSlots()
		reserved = crypto.desk.ReservedSlots()
	}

	if crypto.balance == nil {
		return 0, normal, reserved, errnie.Error(errnie.Err(
			errnie.NotFound,
			"crypto: balance unavailable for plan",
			nil,
		))
	}

	available, err := crypto.balance.AvailableQuote()

	if err != nil {
		return 0, normal, reserved, errnie.Error(errnie.Err(
			errnie.NotFound,
			"crypto: quote balance unavailable for plan",
			err,
		))
	}

	return available, normal, reserved, nil
}

/*
publishStrategy forwards decision frames so the terminal sees strategy output
after Analyzer already published forecasts.
*/
func (crypto *Crypto) publishStrategy(thesis *types.Thesis) {
	if crypto.uiHub == nil || len(thesis.Decisions) == 0 {
		return
	}

	select {
	case crypto.uiHub.Messages <- datura.Map[any]{
		"decisions": thesis.Decisions,
	}.Marshal():
	default:
	}
}
