package broker

import (
	"context"
	"maps"
	"sync"
	"sync/atomic"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/* balanceSnapshot is one complete immutable REST balance observation. */
type balanceSnapshot struct {
	assets map[string]*decimal.Decimal
	from   time.Time
	status types.Status
}

type Balance struct {
	api      *websocket.API
	Quote    string
	snapshot atomic.Pointer[balanceSnapshot]
	Reading  atomic.Pointer[types.EquityReading]
	Failure  atomic.Pointer[error]
	version  atomic.Uint64
	mu       sync.Mutex
}

/* NewBalance binds the existing account and fetches its initial holdings. */
func NewBalance(api *websocket.API) *Balance {
	if api == nil {
		panic("broker: api required")
	}
	balance := &Balance{api: api, Quote: viper.GetViper().GetString("market.quote_currency")}
	balance.Update()
	return balance
}

/* Status reports the latest REST observation's readiness. */
func (balance *Balance) Status() types.Status { return balance.snapshot.Load().status }

/* Update replaces the entire map; readers cannot observe a partially refreshed account. */
func (balance *Balance) Update() {
	from := time.Now().UTC()
	result, err := balance.api.Balance()

	if err != nil {
		failed := balanceSnapshot{status: types.ERROR, from: from}

		if previous := balance.snapshot.Load(); previous != nil {
			failed.assets = previous.assets
		}

		balance.snapshot.Store(&failed)
		errnie.Error(err)

		return
	}

	canonical := make(map[string]*decimal.Decimal, len(result))

	for asset, amount := range result {
		asset = balance.api.Normalizer().Name(asset)

		if _, exists := canonical[asset]; exists {
			failed := balanceSnapshot{status: types.ERROR, from: from}

			if previous := balance.snapshot.Load(); previous != nil {
				failed.assets = previous.assets
			}

			balance.snapshot.Store(&failed)
			errnie.Error(errnie.Err(errnie.Conflict, "balance: multiple venue assets normalize to "+asset, nil))

			return
		}

		canonical[asset] = amount
	}

	balance.snapshot.Store(&balanceSnapshot{assets: canonical, from: from, status: types.READY})
}

/* Assets copies the current account map for callers that retain or transform it. */
func (balance *Balance) Assets() map[string]*decimal.Decimal {
	return maps.Clone(balance.snapshot.Load().assets)
}

/* Cash reports the quote balance exactly as the exchange last stated it. */
func (balance *Balance) Cash() *decimal.Decimal { return balance.snapshot.Load().assets[balance.Quote] }

// Run publishes complete account observations at the existing REST refresh
// cadence. This transport budget does not define a statistical horizon.
func (balance *Balance) Run(ctx context.Context, instrument *Instrument) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		if err := balance.Refresh(instrument); err != nil {
			errnie.Error(err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

/*
Refresh reports what the desk is worth if every open lot were closed now.

Cash alone understates the account while positions are open. Unrealized is the
profit/loss only; equity is cash plus the basis committed to open positions plus
that profit/loss.
*/
func (balance *Balance) Refresh(instrument *Instrument) (err error) {
	if balance == nil || balance.api == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"desk: API required",
			nil,
		))
	}

	balance.mu.Lock()
	defer balance.mu.Unlock()
	defer func() {
		if err != nil {
			balance.Reading.Store(nil)
			balance.Failure.Store(&err)
			return
		}
		balance.Failure.Store(nil)
	}()

	if balance != nil {
		balance.Update()

		if balance.snapshot.Load().status != types.READY {
			return errnie.Error(errnie.Err(errnie.IO, "account: complete wallet observation required", nil))
		}
	}
	tradeBalance, err := balance.api.TradeBalance()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"desk: could not fetch trade balance",
			err,
		))
	}

	reading := types.NewEquityReading(tradeBalance)

	if reading == nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"desk: broker valuation did not include equity",
			nil,
		))
	}

	reading.At, reading.Version = time.Now().UTC(), balance.version.Add(1)
	reading.Positions = make(map[string]string)

	if balance != nil {
		snapshot := balance.snapshot.Load()
		reading.From = snapshot.from
		reading.Complete = reading.Complete && snapshot.status == types.READY
		// A complete total-balance map with no quote holding has zero quote cash.
		reading.Cash = "0"

		if cash := snapshot.assets[balance.Quote]; cash != nil {
			reading.Cash = cash.String()
		}
		for asset, quantity := range snapshot.assets {
			if asset == balance.Quote || quantity.Sign() <= 0 {
				continue
			}
			symbol := asset + "/" + balance.Quote

			if pair := instrument.Pair(symbol); pair.Symbol == symbol {
				reading.Positions[symbol] = quantity.String()
			}
		}
	}

	balance.Reading.Store(reading)

	return nil
}


/*
	NewFundedBalance opens an independent simulated quote account. Positions

own its inventory; live balances continue to be supplied by the venue.
*/
func NewFundedBalance(quote string, cash *decimal.Decimal) (*Balance, error) {
	if quote == "" || cash == nil || cash.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "balance: positive funding and quote currency required", nil))
	}
	balance := &Balance{Quote: quote}
	balance.snapshot.Store(&balanceSnapshot{assets: map[string]*decimal.Decimal{quote: cash}, status: types.READY, from: time.Now()})
	return balance, nil
}

/*
	Settle books one complete simulated fill. The regulator owns cumulative fill

reconciliation; live account observations cannot be modified through this path.
*/
func (balance *Balance) Settle(fill kraken.ExecutionData) error {
	if balance.api != nil || fill.CumCost == nil || fill.FeeUsdEquiv == nil {
		return errnie.Error(errnie.Err(errnie.Validation, "balance: complete simulated fill required", nil))
	}
	balance.mu.Lock()
	defer balance.mu.Unlock()
	cash := balance.Cash().SetScale(decimal.DefaultScale)

	switch fill.Side {
	case "buy":
		cash = cash.Sub(fill.CumCost).Sub(fill.FeeUsdEquiv)
	case "sell":
		cash = cash.Add(fill.CumCost).Sub(fill.FeeUsdEquiv)
	default:
		return errnie.Error(errnie.Err(errnie.Validation, "balance: unknown fill side", nil))
	}

	if cash.Sign() < 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "balance: fill exceeds available cash", nil))
	}
	assets := maps.Clone(balance.snapshot.Load().assets)
	assets[balance.Quote] = cash
	balance.snapshot.Store(&balanceSnapshot{assets: assets, status: types.READY, from: fill.Timestamp})
	return nil
}
