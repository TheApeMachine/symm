package trader

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken/order"
	"github.com/theapemachine/symm/wallet"
)

/*
liveSession routes orders through Kraken WebSocket v2 and reconciles fills.
*/
type liveSession struct {
	client       *order.Client
	router       *broker.Router
	intents      sync.Map
	pendingEntry sync.Map
}

/*
NewLiveSession starts the authenticated trading socket when credentials exist.
*/
func NewLiveSession(ctx context.Context, apiKey, apiSecret string) (*liveSession, error) {
	client, err := order.NewClient(ctx, apiKey, apiSecret)

	if err != nil {
		return nil, err
	}

	session := &liveSession{client: client}
	session.router = broker.NewRouter(func(value any) {
		request, ok := value.(order.Request)

		if !ok {
			return
		}

		_ = client.Publish(request)
	})

	if err := client.Start(); err != nil {
		return nil, err
	}

	return session, nil
}

/*
Router returns the broker router bound to this session.
*/
func (session *liveSession) Router() *broker.Router {
	return session.router
}

/*
Fills exposes the live execution stream.
*/
func (session *liveSession) Fills() <-chan order.Fill {
	return session.client.Fills()
}

/*
Acks exposes trading method responses.
*/
func (session *liveSession) Acks() <-chan order.Ack {
	return session.client.Acks()
}

/*
Close shuts down the live session.
*/
func (session *liveSession) Close() error {
	return session.client.Close()
}

/*
HasPendingEntry reports whether a live entry is already in flight for symbol.
*/
func (session *liveSession) HasPendingEntry(symbol string) bool {
	_, ok := session.pendingEntry.Load(symbol)

	return ok
}

func (session *liveSession) trackEntry(clOrdID, symbol string, intent orderIntent) {
	if clOrdID == "" {
		return
	}

	session.intents.Store(clOrdID, intent)
	session.pendingEntry.Store(symbol, struct{}{})
}

func (session *liveSession) trackExit(clOrdID string, intent orderIntent) {
	if clOrdID == "" {
		return
	}

	session.intents.Store(clOrdID, intent)
}

func (session *liveSession) dropIntent(clOrdID, symbol string) {
	if clOrdID != "" {
		session.intents.Delete(clOrdID)
	}

	if symbol != "" {
		session.pendingEntry.Delete(symbol)
	}
}

func (session *liveSession) intentFor(clOrdID string) (orderIntent, bool) {
	value, ok := session.intents.Load(clOrdID)

	if !ok {
		return orderIntent{}, false
	}

	intent, ok := value.(orderIntent)

	return intent, ok
}

/*
cashDeltaBuy returns quote-currency cash spent on one buy fill.
*/
func cashDeltaBuy(fill order.Fill, quoteCurrency string) float64 {
	cost := fill.Qty * fill.Price

	if fill.Fee <= 0 {
		return cost
	}

	if feeInQuote(fill.FeeCcy, quoteCurrency) {
		return cost + fill.Fee
	}

	return cost
}

/*
cashDeltaSell returns quote-currency proceeds from one sell fill.
*/
func cashDeltaSell(fill order.Fill, quoteCurrency string) float64 {
	proceeds := fill.Qty * fill.Price

	if fill.Fee <= 0 {
		return proceeds
	}

	if feeInQuote(fill.FeeCcy, quoteCurrency) {
		return proceeds - fill.Fee
	}

	return proceeds
}

func feeInQuote(feeCurrency, quoteCurrency string) bool {
	if feeCurrency == "" {
		return true
	}

	return strings.EqualFold(feeCurrency, quoteCurrency)
}

func (session *liveSession) applyBuyFill(
	tradingWallet *wallet.Wallet,
	fill order.Fill,
	intent orderIntent,
) error {
	base := baseOf(fill.Symbol)
	cashDelta := cashDeltaBuy(fill, tradingWallet.Currency)

	if err := tradingWallet.SettleEntryReservation(intent.notional, cashDelta); err != nil {
		return fmt.Errorf("settle entry reservation: %w", err)
	}

	if !tradingWallet.ApplyFill(fill.ExecKey, "buy", base, fill.Qty, fill.Price, cashDelta) {
		return fmt.Errorf("duplicate buy fill %s", fill.ExecKey)
	}

	return nil
}

func (session *liveSession) applySellFill(
	tradingWallet *wallet.Wallet,
	fill order.Fill,
) error {
	base := baseOf(fill.Symbol)
	cashDelta := cashDeltaSell(fill, tradingWallet.Currency)

	if !tradingWallet.ApplyFill(fill.ExecKey, "sell", base, fill.Qty, fill.Price, cashDelta) {
		return fmt.Errorf("duplicate sell fill %s", fill.ExecKey)
	}

	return nil
}

func releaseEntryReservation(tradingWallet *wallet.Wallet, notional float64) {
	if notional > 0 {
		tradingWallet.ReleaseEntryReservation(notional)
	}
}

func liveLotDecimals(symbol string, intent orderIntent) (int, bool) {
	if intent.hasLotDecimals {
		return intent.lotDecimals, true
	}

	return lotDecimals(symbol), lotDecimalsKnown(symbol)
}

func (session *liveSession) handleRejectAck(
	tradingWallet *wallet.Wallet,
	ack order.Ack,
) {
	clOrdID := ack.Result.ClOrdID

	intent, ok := session.intentFor(clOrdID)

	if !ok {
		return
	}

	session.dropIntent(clOrdID, intent.symbol)

	if intent.kind != "entry" {
		return
	}

	releaseEntryReservation(tradingWallet, intent.notional)
}

func liveEnabled(tradingWallet *wallet.Wallet) bool {
	if tradingWallet == nil || tradingWallet.Type != wallet.CryptoWallet {
		return false
	}

	if config.System.KrakenAPIKey == "" || config.System.KrakenAPISecret == "" {
		return false
	}

	return config.System.LiveTradingEnabled
}
