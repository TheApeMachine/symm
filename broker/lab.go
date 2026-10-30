//go:build integration

package broker

import (
	"context"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/nomagique/runtime"
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
	if lab == nil {
		lab = NewLab(context.Background())
	}

	desk := &Desk{
		balance:        balance,
		price:          price,
		positions:      NewCache(lab.ctx),
		virtuals:       NewCache(lab.ctx),
		virtualArchive: &sync.Map{},
		pending:        &sync.Map{},
		exit:           NewExitPolicy(),
	}
	desk.System = runtime.NewSystem(lab.ctx, "desk", desk)
	desk.exit.compose(desk)
	desk.Transition(runtime.READY)
	return desk
}

func (lab *Lab) ReadyPrice(
	books *Book,
	instrument *Instrument,
	normalizer *spot.Normalizer,
	balance *Balance,
) *Price {
	if lab == nil {
		lab = NewLab(context.Background())
	}

	if normalizer == nil {
		normalizer = spot.NewNormalizer()
	}

	price := &Price{
		System:     runtime.NewSystem(lab.ctx, "price"),
		Books:      books,
		Instrument: instrument,
		normalizer: normalizer,
		balance:    balance,
		fees: &sync.Map{},
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
	if desk == nil {
		return
	}

	desk.transport = transport
}

func (instrument *Instrument) RememberPair(pair kraken.InstrumentPair) {
	if instrument == nil || pair.Symbol == "" {
		return
	}

	if instrument.cache == nil {
		instrument.cache = &sync.Map{}
	}

	instrument.cache.Store(pair.Symbol, pair)
}

func (price *Price) RememberTakerFee(symbol string, taker *decimal.Decimal) {
	if price == nil || taker == nil {
		return
	}

	if price.fees == nil {
		price.fees = &sync.Map{}
	}

	price.fees.Store(price.feeName(symbol), taker)
}
