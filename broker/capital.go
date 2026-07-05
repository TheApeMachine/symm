package broker

import (
	"strings"

	"github.com/theapemachine/errnie"
)

/*
Capital reserves quote balance inside one desk update batch.
It does not mutate exchange balances; it only prevents same-batch over-spend.
*/
type Capital struct {
	quote     string
	remaining float64
	loaded    bool
}

/*
NewCapital instantiates a batch capital reserve for the quote asset.
*/
func NewCapital(quote string) *Capital {
	return &Capital{
		quote: strings.ToUpper(strings.TrimSpace(quote)),
	}
}

/*
Reset starts a new desk update batch.
*/
func (capital *Capital) Reset() {
	capital.remaining = 0
	capital.loaded = false
}

/*
Reserve consumes quote balance for a buy order in the current batch.
*/
func (capital *Capital) Reserve(
	order PendingOrder,
	balances *BalanceBook,
) error {
	if strings.ToLower(strings.TrimSpace(order.Side)) != "buy" {
		return nil
	}

	if !capital.loaded {
		cash, err := balances.RequireFunds(capital.quote)

		if err != nil {
			return err
		}

		capital.remaining = cash
		capital.loaded = true
	}

	if order.Notional <= 0 {
		return errnie.Err(
			errnie.Validation,
			"broker: buy order notional required for "+order.Symbol,
			nil,
		)
	}

	if order.Notional > capital.remaining {
		return errnie.Err(
			errnie.NotAcceptable,
			"broker: buy batch exceeds available "+capital.quote+" balance for "+order.Symbol,
			nil,
		)
	}

	capital.remaining -= order.Notional
	return nil
}
