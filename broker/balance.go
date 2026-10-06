package broker

import (
	"context"
	"maps"
	"sync/atomic"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"golang.org/x/time/rate"
)

/*
AccountSnapshot encapsulates the holistic account state (spot assets, quote cash,
portfolio equity, and unrealized PnL) into an immutable, unified record.
All risk and sizing modules evaluate this snapshot to prevent fractured reads.
*/
type AccountSnapshot struct {
	Wallet       *kraken.Balance
	TradeBalance *kraken.TradeBalanceResult
	Assets       map[string]*decimal.Decimal
	Cash         *decimal.Decimal
	Equity       *decimal.Decimal
	Unrealized   *decimal.Decimal
}

/*
Balance is a centralized manager of the exchange wallet, and should be called by
any other object that wants to interact with the balance in any way.
*/
type Balance struct {
	*runtime.System
	transport Transport
	paper     *Paper
	Quote     string
	snapshot  atomic.Pointer[AccountSnapshot]
	stale     chan struct{}
	limiter   *rate.Limiter
}

/*
The venue's own request budgets bound how often Update may run.

Live: Kraken's private REST counter decays by 0.33 per second on the Starter
tier, the lowest tier any account has, and Update spends two points (Balance
and TradeBalance).

Paper: `kraken paper status` prices the wallet through one public Ticker call,
and Kraken's public REST budget is one call per second.
*/
const (
	privateDecayPerSecond = 0.33
	privateCallsPerUpdate = 2
	publicCallsPerSecond  = 1
)

func NewBalance(ctx context.Context, transport Transport) *Balance {
	balance := &Balance{
		System:    runtime.NewSystem(ctx, "balance"),
		transport: transport,
		Quote:     system.Cfg.Market.QuoteCurrency,
		stale:     make(chan struct{}, 1),
		limiter:   rate.NewLimiter(rate.Limit(privateDecayPerSecond/privateCallsPerUpdate), 1),
	}

	if paper, ok := transport.(*Paper); ok {
		balance.paper = paper
		balance.limiter = rate.NewLimiter(rate.Limit(publicCallsPerSecond), 1)
	}

	// Startup needs the venue's account state, so the first read is retried
	// until it succeeds, each attempt spaced by the venue's request budget.
	for {
		if err := balance.limiter.Wait(ctx); err != nil {
			balance.Transition(runtime.ERROR)
			return balance
		}

		err := balance.Update()

		if err == nil {
			break
		}

		errnie.Error(errnie.Err(
			errnie.BadGateway, "[balance] initial account read failed, retrying", err,
		))
	}

	go balance.watch()

	balance.Transition(runtime.READY)
	return balance
}

/*
Invalidate requests a venue refresh after an event that likely changed the
account (a trade print, an execution, a wallet frame). It never blocks: the
refresh runs on the balance's own goroutine, bursts coalesce into one refresh,
and refreshes are spaced by the venue's request budget.
*/
func (balance *Balance) Invalidate() {
	select {
	case balance.stale <- struct{}{}:
	default:
	}
}

func (balance *Balance) watch() {
	for {
		select {
		case <-balance.Context().Done():
			return
		case <-balance.stale:
		}

		if err := balance.limiter.Wait(balance.Context()); err != nil {
			return
		}

		if err := balance.Update(); err != nil {
			errnie.Error(err)
		}
	}
}

func newAccountSnapshot(
	quote string,
	wallet *kraken.Balance,
	tradeBalance *kraken.TradeBalanceResult,
) *AccountSnapshot {
	assets := make(map[string]*decimal.Decimal)
	var cash *decimal.Decimal

	if wallet != nil {
		for _, data := range wallet.Data {
			assets[data.Asset] = data.Balance

			if data.Asset != quote {
				continue
			}

			// Size and enter use spendable cash. Paper (and extended live balances)
			// keep reserved/open-order holds out of Available — Total alone would
			// request ~100 when Available is 0.19.
			if data.Available != nil {
				cash = data.Available
			} else {
				cash = data.Balance
			}
		}
	}

	var equity *decimal.Decimal

	if tradeBalance != nil && tradeBalance.Equity != nil {
		equity = tradeBalance.Equity
	}

	if equity == nil && tradeBalance != nil && tradeBalance.EquivalentBalance != nil {
		equity = tradeBalance.EquivalentBalance
	}

	if equity == nil {
		equity = cash
	}

	var unrealized *decimal.Decimal

	if tradeBalance != nil && tradeBalance.UnrealizedPnL != nil {
		unrealized = tradeBalance.UnrealizedPnL
	}

	if unrealized == nil {
		unrealized = decimal.NewFromInt64(0)
	}

	return &AccountSnapshot{
		Wallet:       wallet,
		TradeBalance: tradeBalance,
		Assets:       assets,
		Cash:         cash,
		Equity:       equity,
		Unrealized:   unrealized,
	}
}

/*
Snapshot loads the latest immutable account state in a single wait-free atomic read.
*/
func (balance *Balance) Snapshot() *AccountSnapshot {
	if balance == nil {
		return nil
	}

	return balance.snapshot.Load()
}

/*
Update refreshes the holistic account state atomically.
*/
func (balance *Balance) Update() error {
	if balance == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[balance] nil balance instance",
			nil,
		))
	}

	if balance.paper != nil {
		wallet, err := balance.paper.Balances()
		if err != nil {
			return errnie.Error(err)
		}

		if wallet == nil {
			return errnie.Error(errnie.Err(
				errnie.IO,
				"[balance] empty paper balance response",
				nil,
			))
		}

		tradeBalance, err := balance.paper.TradeBalance()

		if err != nil {
			return errnie.Error(err)
		}

		balance.snapshot.Store(newAccountSnapshot(balance.Quote, wallet, tradeBalance))
		return nil
	}

	client, err := kraken.NewAuthenticatedREST()

	if err != nil {
		return errnie.Error(err)
	}

	resp, err := client.Balances()
	if err != nil {
		return errnie.Error(err)
	}

	if resp == nil || resp.Result == nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[balance] empty balance response",
			nil,
		))
	}

	wallet := kraken.NewBalanceFromMap(resp.Result)

	// No asset parameter: Kraken values the trade balance in its documented
	// default asset, ZUSD.
	tradeBalance, err := spot.Call[kraken.TradeBalanceResult](client, spot.RequestOptions{
		Auth:   true,
		Method: "POST",
		Path:   system.Cfg.WebSocket.Endpoints.TradeBalance,
	})

	if err != nil {
		return errnie.Error(err)
	}

	balance.snapshot.Store(newAccountSnapshot(balance.Quote, wallet, &tradeBalance.Result))
	return nil
}

/*
Assets copies the current account map for callers that retain or transform it.
*/
func (balance *Balance) Assets() map[string]*decimal.Decimal {
	snapshot := balance.Snapshot()

	if snapshot == nil || snapshot.Assets == nil {
		return make(map[string]*decimal.Decimal)
	}

	out := make(map[string]*decimal.Decimal, len(snapshot.Assets))
	maps.Copy(out, snapshot.Assets)

	return out
}

/* Cash reports spendable quote funds (Available when present, else Balance). */
func (balance *Balance) Cash() *decimal.Decimal {
	snapshot := balance.Snapshot()

	if snapshot == nil {
		return nil
	}

	return snapshot.Cash
}

/* Equity returns total portfolio equity from authoritative trade balance or cash. */
func (balance *Balance) Equity() *decimal.Decimal {
	snapshot := balance.Snapshot()

	if snapshot == nil {
		return nil
	}

	return snapshot.Equity
}

/* Unrealized returns unrealized profit/loss across all open positions. */
func (balance *Balance) Unrealized() *decimal.Decimal {
	snapshot := balance.Snapshot()

	if snapshot == nil {
		return decimal.NewFromInt64(0)
	}

	return snapshot.Unrealized
}

/*
UpdateWallet applies a streamed wallet frame. The streamed frame carries no
trade balance, so the last one the venue reported is kept until the next
refresh replaces it.

A "snapshot" frame replaces the wallet. An "update" frame carries ledger
entries for the assets that changed, each with that asset's new total, so it
is merged into the held wallet: assets it does not mention keep their last
balance, and a changed asset's Available/Reserved split is dropped (the
ledger entry does not carry it) so Cash falls back to the new total. Any
other frame type, or an update with no held wallet to merge into, is an
error: the account state is never replaced with a partial view.
*/
func (balance *Balance) UpdateWallet(wallet *kraken.Balance) error {
	if balance == nil {
		return errnie.Err(errnie.Validation, "[balance] nil balance instance", nil)
	}

	if wallet == nil {
		return errnie.Err(errnie.Validation, "[balance] nil wallet frame", nil)
	}

	var (
		tradeBalance *kraken.TradeBalanceResult
		held         *kraken.Balance
	)

	if current := balance.snapshot.Load(); current != nil {
		tradeBalance = current.TradeBalance
		held = current.Wallet
	}

	switch wallet.Type {
	case "snapshot":
	case "update":
		if held == nil {
			return errnie.Err(
				errnie.Validation,
				"[balance] wallet update without a held wallet snapshot",
				nil,
			)
		}

		merged, err := mergeWalletUpdate(held, wallet)

		if err != nil {
			return err
		}

		wallet = merged
	default:
		return errnie.Err(
			errnie.UnprocessableContent,
			"[balance] unknown wallet frame type "+wallet.Type,
			nil,
		)
	}

	balance.snapshot.Store(newAccountSnapshot(balance.Quote, wallet, tradeBalance))
	return nil
}

/*
mergeWalletUpdate returns a new wallet: held with every asset named in update
replaced by its new total. held is never mutated; snapshots are immutable.
*/
func mergeWalletUpdate(held, update *kraken.Balance) (*kraken.Balance, error) {
	changed := make(map[string]kraken.BalanceData, len(update.Data))
	order := make([]string, 0, len(update.Data))

	for _, entry := range update.Data {
		if entry.Asset == "" {
			return nil, errnie.Err(
				errnie.UnprocessableContent,
				"[balance] wallet update entry without asset",
				nil,
			)
		}

		if entry.Balance == nil {
			return nil, errnie.Err(
				errnie.UnprocessableContent,
				"[balance] wallet update without balance for "+entry.Asset,
				nil,
			)
		}

		if _, seen := changed[entry.Asset]; !seen {
			order = append(order, entry.Asset)
		}

		// Ledger entries within one frame are in venue order; the last one
		// for an asset carries its final total.
		changed[entry.Asset] = kraken.BalanceData{
			Asset:      entry.Asset,
			AssetClass: entry.AssetClass,
			Balance:    entry.Balance,
		}
	}

	merged := &kraken.Balance{
		Channel:   update.Channel,
		Type:      "snapshot",
		Sequence:  update.Sequence,
		Timestamp: update.Timestamp,
		Data:      make([]kraken.BalanceData, 0, len(held.Data)+len(changed)),
	}

	for _, entry := range held.Data {
		if replacement, ok := changed[entry.Asset]; ok {
			merged.Data = append(merged.Data, replacement)
			delete(changed, entry.Asset)
			continue
		}

		merged.Data = append(merged.Data, entry)
	}

	for _, asset := range order {
		if entry, ok := changed[asset]; ok {
			merged.Data = append(merged.Data, entry)
		}
	}

	return merged, nil
}

/*
EquityWire renders the current account state for the dashboard top bar.
*/
func (balance *Balance) EquityWire() *wire.EquityFrameT {
	snapshot := balance.Snapshot()

	if snapshot == nil || snapshot.Cash == nil || snapshot.Equity == nil {
		return nil
	}

	return &wire.EquityFrameT{
		Cash:       snapshot.Cash.String(),
		Unrealized: snapshot.Unrealized.String(),
		Equity:     snapshot.Equity.String(),
	}
}
