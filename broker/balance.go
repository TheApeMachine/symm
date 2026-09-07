package broker

import (
	"context"
	"sync"

	"maps"
	"sort"
	"sync/atomic"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken/websocket"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

/* balanceSnapshot is one complete immutable REST balance observation. */
type balanceSnapshot struct {
	assets map[string]*decimal.Decimal
	from   time.Time
	status types.Status
}

/*
	Balance publishes complete exchange-owned wallet maps atomically. Reservations

belong to execution admission and never modify these authoritative balances.
*/
type cashReservation struct {
	cost     *decimal.Decimal
	terminal time.Time
}

type Balance struct {
	reservations map[string]cashReservation
	api          *websocket.API
	Quote        string
	snapshot     atomic.Pointer[balanceSnapshot]
	Reading      atomic.Pointer[types.EquityReading]
	Failure      atomic.Pointer[error]
	version      atomic.Uint64
	mu           sync.Mutex
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

/* Wallet publishes the same authoritative assets used by execution and recovery. */
func (balance *Balance) Wallet() *types.UIFrame {
	snapshot := balance.snapshot.Load()
	balances := make([]*wire.BalanceT, 0, len(snapshot.assets))
	for asset, amount := range snapshot.assets {
		balances = append(balances, &wire.BalanceT{Asset: asset, Amount: amount.String()})
	}
	sort.Slice(balances, func(left, right int) bool { return balances[left].Asset < balances[right].Asset })
	return &wire.FrameT{Type: wire.FrameBalancesFrame, Value: &wire.BalancesFrameT{Balances: balances}}
}

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
	for identity, reservation := range balance.reservations {
		if !reservation.terminal.IsZero() && reading.From.After(reservation.terminal) {
			delete(balance.reservations, identity)
		}
	}
	balance.Reading.Store(reading)

	return nil
}

/* Reserve atomically charges a commitment against the latest authoritative cash. */
func (balance *Balance) Reserve(identity string, cost *decimal.Decimal) bool {
	balance.mu.Lock()
	defer balance.mu.Unlock()
	reading := balance.Reading.Load()

	if reading == nil || !reading.Complete || cost.Sign() <= 0 {
		return false
	}

	if _, found := balance.reservations[identity]; found {
		return false
	}
	available, err := decimal.NewFromString(reading.AvailableCash)

	if err != nil {
		errnie.Error(errnie.Err(errnie.Validation, "balance: invalid available cash", err))
		return false
	}

	for _, reservation := range balance.reservations {
		available = available.Sub(reservation.cost)
	}

	if available.Cmp(cost) < 0 {
		return false
	}

	if balance.reservations == nil {
		balance.reservations = make(map[string]cashReservation)
	}
	balance.reservations[identity] = cashReservation{cost: cost}
	return true
}

/* Release retains submitted commitments until a causally later balance arrives. */
func (balance *Balance) Release(identity string, terminal time.Time) {
	balance.mu.Lock()
	defer balance.mu.Unlock()
	reservation, found := balance.reservations[identity]

	if !found {
		return
	}

	if terminal.IsZero() {
		delete(balance.reservations, identity)
		return
	}
	reservation.terminal = terminal
	balance.reservations[identity] = reservation
}

/* Committed returns the total still charged to locally submitted orders. */
func (balance *Balance) Committed() *decimal.Decimal {
	balance.mu.Lock()
	defer balance.mu.Unlock()
	committed := decimalZero

	for _, reservation := range balance.reservations {
		committed = committed.Add(reservation.cost)
	}
	return committed
}
