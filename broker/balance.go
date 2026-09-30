package broker

import (
	"context"
	"maps"
	"sync/atomic"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/network"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
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
	private  *network.WebsocketClient
	Quote    string
	snapshot atomic.Pointer[AccountSnapshot]
}

func NewBalance(ctx context.Context, private *network.WebsocketClient) *Balance {
	balance := &Balance{
		System:  runtime.NewSystem(ctx, "balance"),
		private: private,
		Quote:   system.Cfg.Market.QuoteCurrency,
	}

	balance.Update()
	return balance
}

func newAccountSnapshot(
	quote string,
	wallet *kraken.Balance,
	tradeBalance *kraken.TradeBalanceResult,
) *AccountSnapshot {
	assets := make(map[string]*decimal.Decimal)

	if wallet != nil {
		for _, data := range wallet.Data {
			assets[data.Asset] = data.Balance
		}
	}

	var cash *decimal.Decimal

	if quoteAmount, found := assets[quote]; found {
		cash = quoteAmount
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
func (balance *Balance) Update() {
	if balance == nil {
		return
	}
	// Websocket router calls UpdateWallet
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

/* Cash reports the quote balance exactly as the exchange last stated it. */
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

func (balance *Balance) UpdateWallet(wallet *kraken.Balance) {
	if balance == nil || wallet == nil {
		return
	}
	snapshot := newAccountSnapshot(balance.Quote, wallet, nil)
	balance.snapshot.Store(snapshot)
}
