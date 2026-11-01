//go:build integration

package broker

import (
	"context"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/symm/decimal"
	"github.com/theapemachine/symm/kraken"
)

/*
Lab wires an isolated desk and price for integration tests. Nothing here ships
in the trading binary; build with -tags=integration.
*/

type Lab struct {
	ctx context.Context
}

func NewLab(ctx context.Context) *Lab {
	return &Lab{ctx: ctx}
}

func (lab *Lab) Desk(price *Price, balance *Balance) *Desk {
	return NewDesk(
		lab.ctx,
		map[string]Transport{},
		price,
		map[string]*Balance{
			cachePaper: balance,
			cacheReal:  balance,
		},
	)
}

func (lab *Lab) ReadyPrice(
	books *Book,
	instrument *Instrument,
	normalizer *spot.Normalizer,
	balance *Balance,
) *Price {
	if normalizer == nil {
		normalizer = spot.NewNormalizer()
	}

	price := &Price{
		System:     runtime.NewSystem(lab.ctx, "price"),
		Books:      books,
		Tape:       NewTape(),
		Instrument: instrument,
		normalizer: normalizer,
		balance:    balance,
		fees:       &sync.Map{},
	}
	price.Transition(runtime.READY)
	return price
}

func (lab *Lab) StaticBalance(quote string, amount *decimal.Decimal) *Balance {
	balance := &Balance{Quote: quote}
	balance.wallet.Store(&kraken.Balance{Data: []kraken.BalanceData{{
		Asset:     quote,
		Balance:   amount,
		Available: amount,
	}}})
	return balance
}

func (desk *Desk) AttachTransport(transport Transport) {
	desk.transports[cachePaper] = transport
	desk.transports[cacheReal] = transport
}
